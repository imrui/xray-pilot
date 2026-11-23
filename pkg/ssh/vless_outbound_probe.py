# public_probe_target is shared with the deployment health check.
import json
import os
import queue
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


def bounded_lookup(lookup, timeout):
    # getaddrinfo 本身不接受超时。解析线程仅负责 DNS，不会在超时后再拨号。
    answer = queue.Queue(maxsize=1)

    def resolve():
        try:
            answer.put(lookup())
        except (OSError, ValueError, OverflowError):
            answer.put(None)

    threading.Thread(target=resolve, daemon=True).start()
    try:
        return answer.get(timeout=timeout)
    except queue.Empty:
        return None


def fields(obj):
    if not isinstance(obj, dict):
        raise ProbeError("ENDPOINT")
    result = {key.lower(): value for key, value in obj.items()}
    if len(result) != len(obj):
        raise ProbeError("ENDPOINT")
    return result


def relay_endpoint(outbound):
    ob = fields(outbound)
    stream = fields(ob.get("streamsettings", {}))
    network = stream.get("method") or stream.get("network") or "tcp"
    if not isinstance(network, str) or network.lower() not in ("tcp", "raw", "ws", "websocket", "grpc", "httpupgrade"):
        raise ProbeError("TRANSPORT")
    # 普通 socket 无法复现出站的源地址、路由标记、网卡、浏览器转发等特殊拨号设置。
    sockopt = fields(stream.get("sockopt") or {})
    local_options = {"tcpkeepaliveidle", "tcpkeepaliveinterval", "tcpnodelay", "tcpusertimeout", "tcpcongestion"}
    if str(sockopt.get("domainstrategy", "")).lower() in ("", "asis"):
        sockopt.pop("domainstrategy", None)
    if (str(ob.get("targetstrategy", "")).lower() not in ("", "asis") or
            any(value for key, value in sockopt.items() if key not in local_options) or
            ob.get("sendthrough") or stream.get("finalmask") or stream.get("address") or stream.get("port") or
            fields(stream.get("wssettings", {})).get("usebrowserforwarding") or
            fields(stream.get("httpupgradesettings", {})).get("usebrowserforwarding")):
        raise ProbeError("ROUTE")
    settings = fields(ob.get("settings"))
    if settings.get("vnext"):
        servers = settings["vnext"]
        if settings.get("address") or not isinstance(servers, list) or len(servers) != 1:
            raise ProbeError("ENDPOINT")
        settings = fields(servers[0])
    host, port = settings.get("address"), settings.get("port")
    if not isinstance(host, str) or not host or type(port) is not int or not 1 <= port <= 65535:
        raise ProbeError("ENDPOINT")
    if host.startswith("[") and host.endswith("]"):
        try:
            host = str(ipaddress.IPv6Address(host[1:-1]))
        except ValueError:
            raise ProbeError("ENDPOINT")
    return host, port


def tcp_rtt(outbound):
    host, port = relay_endpoint(outbound)
    deadline = time.monotonic() + 4
    addresses = bounded_lookup(lambda: socket.getaddrinfo(host, port, type=socket.SOCK_STREAM), 4)
    if not addresses:
        raise ProbeError("RESOLVE")
    for family, kind, proto, _, address in addresses:
        left = deadline - time.monotonic()
        if left <= 0:
            break
        try:
            with socket.socket(family, kind, proto) as sock:
                sock.settimeout(min(left, 2))
                # 只统计成功地址的 TCP 握手，排除 DNS 和先前地址的失败等待。
                start = time.monotonic()
                sock.connect(address)
                return round((time.monotonic() - start) * 1_000_000)
        except OSError:
            continue
    raise ProbeError("CONNECT")


def probe(binary, outbound, url):
    # 原 HTTPS 探测最多运行 30 秒，包括 DNS 切换备用服务器所需的等待。
    deadline = time.monotonic() + 30
    public = bounded_lookup(lambda: public_probe_target(url), 30)
    if public is None:
        raise ProbeError("TARGET")
    host, target = public
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
        if not ready.wait(max(0, min(4, deadline - time.monotonic()))) or child.poll() is not None:
            raise ProbeError("START")
        start = time.monotonic()
        request_timeout = min(14, deadline - start)
        if request_timeout <= 0:
            raise ProbeError("HTTPS")
        # -q must be first. Pin the public probe IP, keep the hostname for TLS
        # verification, disallow proxy bypass, and keep credentials off argv.
        result = subprocess.run(
            ["curl", "-q", "--silent", "--noproxy", "", "--proto", "=https",
             "--socks5-hostname", "127.0.0.1:" + str(port), "--config", "-",
             "--connect-to", host + ":443:" + target + ":443", "--max-time", "12",
             "--output", os.devnull, "--write-out", "%{http_code}", "--", url],
            input=('proxy-user = "probe:' + password + '"\n').encode(),
            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=request_timeout)
        if child.poll() is not None or result.returncode != 0 or not re.fullmatch(rb"[1-5][0-9]{2}", result.stdout):
            raise ProbeError("HTTPS")
        if not result.stdout.startswith(b"2"):
            raise ProbeError("HTTP " + result.stdout.decode("ascii"))
        return round((time.monotonic() - start) * 1000)
    except FileNotFoundError:
        raise ProbeError("TOOLS")
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
        try:
            https = "OK " + str(probe(binary, outbound, url))
        except ProbeError as err:
            https = str(err)
        except OSError:
            https = "START"
        try:
            tcp = "OK " + str(tcp_rtt(outbound))
        except ProbeError as err:
            tcp = str(err)
        print("TCP", tcp)
        print(https)
    except ProbeError as err:
        print(str(err))
    except OSError:
        print("START")
