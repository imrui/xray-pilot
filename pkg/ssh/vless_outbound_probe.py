# public_probe_target is shared with the deployment health check.
import json
import os
import re
import secrets
import signal
import subprocess
import sys
import threading
import time


class ProbeError(Exception):
    pass


def prepare_outbound(obj, custom_dns):
    if isinstance(obj, list):
        for item in obj:
            prepare_outbound(item, custom_dns)
    elif isinstance(obj, dict):
        for key, value in obj.items():
            key = key.lower()
            if key == "headers":
                continue
            if key in ("dialerproxy", "proxysettings") and value:
                raise ProbeError("DEPENDENCY")
            if custom_dns and key in ("domainstrategy", "targetstrategy") and str(value).lower() not in ("", "asis"):
                raise ProbeError("DEPENDENCY")
            if key in ("tlssettings", "realitysettings") and isinstance(value, dict):
                for field in list(value):
                    if field.lower() == "masterkeylog":
                        del value[field]
            prepare_outbound(value, custom_dns)


def select_outbound(path, tag):
    try:
        with open(path, "rb") as source:
            raw = source.read((16 << 20) + 1)
        if len(raw) > 16 << 20:
            raise ProbeError("CONFIG")
        cfg = json.loads(raw)
        matches = [o for o in cfg["outbounds"] if o.get("tag") == tag]
        if len(matches) != 1 or matches[0].get("protocol") != "vless":
            raise ProbeError("MISSING")
        prepare_outbound(matches[0], bool(cfg.get("dns")))
        return matches[0]
    except (ValueError, OSError, KeyError, TypeError, AttributeError):
        raise ProbeError("CONFIG")


def probe(binary, outbound, url):
    try:
        host, target = public_probe_target(url)
    except (ValueError, OSError):
        raise ProbeError("TARGET")
    # The chosen port may be taken after bind. Only the new child's own startup
    # receipt permits the request, and the SOCKS listener requires a one-use password.
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    password = secrets.token_hex(24)
    config = {
        "log": {"loglevel": "warning"},
        "inbounds": [{"listen": "127.0.0.1", "port": port, "protocol": "socks",
                      "settings": {"auth": "password", "accounts": [{"user": "probe", "pass": password}]}}],
        "outbounds": [outbound],
    }
    child = subprocess.Popen([binary, "run", "-format", "json", "-config", "stdin:"],
                             stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    ready = threading.Event()

    def read_log():
        for line in child.stdout:
            if re.search(rb"\[Warning\] core: Xray [^ ]+ started\s*$", line):
                ready.set()

    reader = threading.Thread(target=read_log, daemon=True)
    reader.start()
    try:
        with child.stdin:
            child.stdin.write(json.dumps(config).encode())
        if not ready.wait(4) or child.poll() is not None:
            raise ProbeError("START")
        start = time.monotonic()
        # -q must be first. Pin the public probe IP, keep the hostname for TLS
        # verification, disallow proxy bypass, and keep credentials off argv.
        result = subprocess.run(
            ["curl", "-q", "--silent", "--noproxy", "", "--proto", "=https",
             "--socks5-hostname", "127.0.0.1:" + str(port), "--config", "-",
             "--connect-to", host + ":443:" + target + ":443", "--max-time", "12",
             "--output", os.devnull, "--write-out", "%{http_code}", "--", url],
            input=('proxy-user = "probe:' + password + '"\n').encode(),
            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=14)
        if child.poll() is not None or result.returncode != 0 or not re.fullmatch(rb"[1-5][0-9]{2}", result.stdout):
            raise ProbeError("HTTPS")
        if not result.stdout.startswith(b"2"):
            raise ProbeError("HTTP " + result.stdout.decode("ascii"))
        return round((time.monotonic() - start) * 1000)
    except subprocess.TimeoutExpired:
        raise ProbeError("HTTPS")
    finally:
        if child.poll() is None:
            child.terminate()
            try:
                child.wait(timeout=1)
            except subprocess.TimeoutExpired:
                child.kill()
                child.wait()
        reader.join()
        child.stdout.close()


def interrupted(signum, frame):
    raise ProbeError("INTERRUPTED")


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, interrupted)
    try:
        path, tag, url, binary = sys.argv[1:]
        outbound = select_outbound(path, tag)
        print("OK", probe(binary, outbound, url))
    except ProbeError as err:
        print(str(err))
    except OSError:
        print("START")
