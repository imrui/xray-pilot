package ssh

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
)

const DefaultProbeURL = "https://www.gstatic.com/generate_204"

func ValidateProbeURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.Port() != "" && u.Port() != "443" {
		return fmt.Errorf("健康探测须使用不含账号、查询参数的 HTTPS 公网 URL（443 端口）")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && (!ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback()) {
		return fmt.Errorf("健康探测目标不能是私网地址")
	}
	return nil
}

// probeEnvelope travels only over SSH stdin; credentials are never command arguments.
func probeEnvelope(content, probeURL string) (string, error) {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		return "", err
	}
	probes := []any{}
	ports := []any{}
	inbounds, _ := cfg["inbounds"].([]any)
	for _, raw := range inbounds {
		in, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		protocol, _ := in["protocol"].(string)
		if protocol == "dokodemo-door" {
			continue
		}
		ports = append(ports, in["port"])
		settings, _ := in["settings"].(map[string]any)
		clients, _ := settings["clients"].([]any)
		if (protocol != "vless" && protocol != "trojan") || len(clients) == 0 {
			continue
		}
		user, ok := clients[0].(map[string]any)
		if !ok {
			continue
		}
		// JSON unmarshal creates independent objects, so the server config isn't mutated.
		rawStream, _ := json.Marshal(in["streamSettings"])
		stream := map[string]any{}
		if json.Unmarshal(rawStream, &stream) != nil {
			continue
		}
		switch stream["security"] {
		case "reality":
			rs, _ := stream["realitySettings"].(map[string]any)
			priv, _ := rs["privateKey"].(string)
			bytes, err := base64.RawURLEncoding.DecodeString(priv)
			if err != nil {
				return "", fmt.Errorf("Reality 私钥格式无效")
			}
			key, err := ecdh.X25519().NewPrivateKey(bytes)
			if err != nil {
				return "", fmt.Errorf("Reality 私钥无效")
			}
			names, _ := rs["serverNames"].([]any)
			shortIDs, _ := rs["shortIds"].([]any)
			if len(names) == 0 || len(shortIDs) == 0 {
				return "", fmt.Errorf("Reality 缺少 SNI 或 shortId")
			}
			stream["realitySettings"] = map[string]any{"serverName": names[0], "publicKey": base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), "shortId": shortIDs[0], "fingerprint": "chrome"}
		case "tls":
			tls, _ := stream["tlsSettings"].(map[string]any)
			if tls == nil {
				return "", fmt.Errorf("缺少 TLS 配置")
			}
			delete(tls, "certificates")
			tls["allowInsecure"] = false
		}
		account := map[string]any{"id": user["id"], "encryption": "none"}
		if flow, ok := user["flow"]; ok {
			account["flow"] = flow
		}
		server := map[string]any{"address": "127.0.0.1", "port": in["port"]}
		outbound := map[string]any{"protocol": protocol, "streamSettings": stream}
		if protocol == "vless" {
			server["users"] = []any{account}
			outbound["settings"] = map[string]any{"vnext": []any{server}}
		} else {
			server["password"] = user["password"]
			outbound["settings"] = map[string]any{"servers": []any{server}}
		}
		probes = append(probes, map[string]any{"log": map[string]any{"loglevel": "none"}, "outbounds": []any{outbound}})
	}
	if probeURL == "" {
		probeURL = DefaultProbeURL
	}
	if err := ValidateProbeURL(probeURL); err != nil {
		return "", err
	}
	data, err := json.Marshal(map[string]any{"config": cfg, "probes": probes, "ports": ports, "probe_url": probeURL})
	return string(data), err
}

// The target is validated after node-side DNS resolution and redirects are not followed.
const proxyHealthPython = `import ipaddress,json,socket,subprocess,sys,tempfile,time,os,urllib.parse
data=json.load(open(sys.argv[1]))
url=data["probe_url"]
host=urllib.parse.urlparse(url).hostname
answers=socket.getaddrinfo(host,443,type=socket.SOCK_STREAM)
if not answers or any(not ipaddress.ip_address(a[4][0]).is_global for a in answers): sys.exit(2)
for port in data["ports"]:
    with socket.create_connection(("127.0.0.1",int(port)),timeout=2): pass
probes=data["probes"]
if not probes: sys.exit(0) # legacy configurations without accounts retain basic checking
target=next((a[4][0] for a in answers if a[0]==socket.AF_INET),answers[0][4][0])
if ":" in target: target="["+target+"]"
for probe in probes:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1",0)); port=sock.getsockname()[1]
    probe["inbounds"]=[{"listen":"127.0.0.1","port":port,"protocol":"socks","settings":{"auth":"noauth"}}]
    fd,path=tempfile.mkstemp(prefix=".pilot-probe.",dir=os.path.dirname(sys.argv[1]))
    child=None
    try:
        with os.fdopen(fd,"w") as f: json.dump(probe,f)
        child=subprocess.Popen([sys.argv[2],"run","-config",path],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        deadline=time.monotonic()+3
        while True:
            try:
                with socket.create_connection(("127.0.0.1",port),timeout=.2): break
            except OSError:
                if time.monotonic()>deadline: raise
                time.sleep(.1)
        subprocess.run(["curl","--silent","--fail","--max-time","6","--socks5-hostname","127.0.0.1:"+str(port),"--connect-to",host+":443:"+target+":443",url],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,check=True,timeout=8)
    finally:
        if child is not None:
            child.terminate()
            try: child.wait(timeout=2)
            except subprocess.TimeoutExpired: child.kill(); child.wait()
        os.unlink(path)
`
