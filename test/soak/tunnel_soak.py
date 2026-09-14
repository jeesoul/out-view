"""真实 Java + Go 回环验收；所有数据库、日志和进程都属于本次临时运行。"""
import argparse
import base64
from concurrent.futures import ThreadPoolExecutor
import json
import os
from pathlib import Path
import socket
import socketserver
import struct
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--jar", type=Path, default=ROOT / "target/outview-server.jar")
    parser.add_argument("--client", type=Path, required=True, help="本次构建的 CLI 路径")
    parser.add_argument("--java", default="java")
    parser.add_argument("--idle-seconds", type=int, default=120, help="同一转发连接空闲时间，默认超过90秒心跳阈值")
    args = parser.parse_args()
    if args.idle_seconds < 0 or not args.jar.is_file() or not args.client.is_file():
        parser.error("需要有效的 JAR/CLI 文件和非负空闲时长")
    run = Path(tempfile.mkdtemp(prefix="outview-soak-"))
    events, processes, logs = [], [], []
    auth = "Basic " + base64.b64encode(b"scan:outview-test-only").decode()
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def record(name, **data):
        event = dict(check=name, **data)
        events.append(event)
        print(json.dumps(event, ensure_ascii=True), flush=True)
        (run / "results.json").write_text(json.dumps(events, indent=2, ensure_ascii=False), encoding="utf-8")

    def free_port():
        with socket.socket() as probe:
            probe.bind(("127.0.0.1", 0))
            return probe.getsockname()[1]

    http_port, control_port = free_port(), free_port()
    # 独占一个连续端口范围用于固定预留及冲突测试。
    for start in range(24000, 29000, 10):
        probes = []
        try:
            for port in range(start, start + 6):
                probe = socket.socket()
                probes.append(probe)
                probe.bind(("127.0.0.1", port))
            break
        except OSError:
            continue
        finally:
            for probe in probes:
                probe.close()
    else:
        raise RuntimeError("没有可用的回环测试端口范围")
    fixed, new_port = start + 1, start + 2

    def request(path, method="GET", data=None):
        headers = {"Accept": "application/json", "Authorization": auth}
        if data is not None:
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(f"http://127.0.0.1:{http_port}{path}", method=method,
                                     data=json.dumps(data).encode() if data is not None else None, headers=headers)
        with opener.open(req, timeout=3) as response:
            return json.load(response)

    def wait_for(predicate, timeout=40):
        deadline = time.monotonic() + timeout
        last_error = None
        while time.monotonic() < deadline:
            try:
                result = predicate()
                if result:
                    return result
            except (OSError, urllib.error.URLError) as error:
                last_error = error
            time.sleep(0.1)
        raise AssertionError(f"等待状态超时: {last_error}")

    def spawn(command, name):
        log = open(run / (name + ".log"), "w", encoding="utf-8")
        logs.append(log)
        env = dict(os.environ, OUTVIEW_DATA_DIR=(run / "data").as_posix())
        proc = subprocess.Popen(command, cwd=run, env=env, stdout=log, stderr=subprocess.STDOUT,
                                creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0)
        processes.append(proc)
        return proc

    def stop(proc):
        if proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=5)

    def start_server(name):
        proc = spawn([args.java, "-Xmx256m", "-jar", str(args.jar.resolve()),
                      f"--server.port={http_port}", "--server.address=127.0.0.1",
                      "--outview.bind-address=127.0.0.1", f"--outview.control-port={control_port}",
                      f"--outview.data-port-start={start}", f"--outview.data-port-end={start + 5}",
                      "--outview-admin.users[0].username=scan", "--outview-admin.users[0].password=outview-test-only",
                      "--logging.level.com.outview=INFO"], name)
        wait_for(lambda: request("/health").get("status") == "UP")
        return proc

    def online():
        return next((d for d in request("/api/devices")["devices"] if d["deviceId"] == "123456"), None)

    def read_exact(conn, size):
        result = bytearray()
        while len(result) < size:
            part = conn.recv(size - len(result))
            if not part:
                raise AssertionError("转发连接提前关闭")
            result.extend(part)
        return bytes(result)

    def roundtrip(conn, payload):
        conn.sendall(payload)
        assert read_exact(conn, len(payload)) == payload, "数据内容不一致"

    def query():
        body = b'{"deviceCode":"123456"}'
        with socket.create_connection(("127.0.0.1", control_port), timeout=5) as conn:
            conn.sendall(struct.pack("!IBBIH", 0x4F565753, 1, 14, len(body), 0) + body)
            _, _, kind, size, _ = struct.unpack("!IBBIH", read_exact(conn, 12))
            assert kind == 15
            return json.loads(read_exact(conn, size))["externalPort"]

    class Echo(socketserver.BaseRequestHandler):
        def handle(self):
            try:
                while data := self.request.recv(65536):
                    self.request.sendall(data)
            except (ConnectionError, OSError):
                pass  # 故障注入会主动中断连接。

    class Server(socketserver.ThreadingTCPServer):
        daemon_threads = True

    echo = Server(("127.0.0.1", 0), Echo)
    threading.Thread(target=echo.serve_forever, daemon=True).start()
    success = False
    try:
        server = start_server("server-first")
        result = request("/api/devices/mappings", "POST", {
            "deviceId": "123456", "externalPort": fixed, "targetPort": echo.server_address[1]})
        assert result["success"], result
        client = spawn([str(args.client.resolve()), "-host", "127.0.0.1", "-port", str(control_port),
                        "-device-id", "123456", "-token", "scan-token",
                        "-local-port", str(echo.server_address[1])], "client")
        assert wait_for(online)["externalPort"] == fixed
        assert query() == fixed
        record("registered_fixed_port", port=fixed, http_port=http_port, evidence=str(run))
        with socket.create_connection(("127.0.0.1", fixed), timeout=10) as conn:
            roundtrip(conn, b"before-idle")
            deadline = time.monotonic() + args.idle_seconds
            next_report = time.monotonic() + 15
            while time.monotonic() < deadline:
                time.sleep(min(1, max(0, deadline - time.monotonic())))
                assert client.poll() is None and server.poll() is None, "进程异常退出"
                if time.monotonic() >= next_report:
                    record("idle_progress", remaining_seconds=round(max(0, deadline - time.monotonic())))
                    next_report += 15
            roundtrip(conn, bytes(range(256)) * 256)
        record("idle_connection_survived", seconds=args.idle_seconds, verified_bytes=65536)

        def transfer(number):
            total = 0
            with socket.create_connection(("127.0.0.1", fixed), timeout=10) as conn:
                for sequence in range(20):
                    payload = bytes([number, sequence]) * 32768
                    roundtrip(conn, payload)
                    total += len(payload)
            return total

        with ThreadPoolExecutor(max_workers=4) as pool:
            total = sum(pool.map(transfer, range(4)))
        record("concurrent_transfer", connections=4, verified_bytes=total)
        # 先制造真实端口冲突，确认失败不会损坏旧映射。
        with socket.socket() as occupied:
            occupied.bind(("127.0.0.1", new_port))
            occupied.listen()
            result = request("/api/devices/mappings/123456", "PUT", {"externalPort": new_port})
            assert not result["success"] and query() == fixed
            with socket.create_connection(("127.0.0.1", fixed), timeout=5) as conn:
                roundtrip(conn, b"old-port-still-works")
        record("port_conflict_preserves_old_mapping", port=fixed)
        result = request("/api/devices/mappings/123456", "PUT", {"externalPort": new_port})
        assert result["success"] and query() == new_port
        assert online()["externalPort"] == new_port
        with socket.create_connection(("127.0.0.1", new_port), timeout=5) as conn:
            roundtrip(conn, b"new-port-works")
        record("online_port_change", port=new_port, query_and_session_consistent=True)
        # Windows 上 terminate 是强制退出；立即重启验证实际文件数据库的固定预留。
        stop(server)
        server = start_server("server-restarted")
        assert wait_for(online)["externalPort"] == new_port
        with socket.create_connection(("127.0.0.1", new_port), timeout=5) as conn:
            roundtrip(conn, b"recovered-after-server-restart")
        record("server_restart_and_auto_reconnect", persisted_port=new_port)
        result = request("/api/devices/123456/ban", "POST", {"reason": "temporary test"})
        assert result["success"]
        wait_for(lambda: not online())
        assert request("/api/devices/mappings")["mappings"][0]["externalPort"] == new_port
        record("ban_retains_reservation", port=new_port)
        success = True
    finally:
        for proc in reversed(processes):
            stop(proc)
        echo.shutdown()
        echo.server_close()
        for log in logs:
            log.close()
        record("cleanup", success=success, all_processes_stopped=all(p.poll() is not None for p in processes))


if __name__ == "__main__":
    main()
