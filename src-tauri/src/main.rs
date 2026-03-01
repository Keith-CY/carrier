use std::io::{Read, Write};
use std::net::{SocketAddr, TcpStream};
use std::process::{Command, Stdio};
use std::str::FromStr;
use std::thread::sleep;
use std::time::{Duration, Instant};
use tauri::Manager;

const CARRIER_DAEMON_ADDR: &str = "127.0.0.1:9090";
const CARRIER_GATEWAY_ADDR: &str = "127.0.0.1:8787";
const CARRIER_GATEWAY_URL: &str = "http://127.0.0.1:8787/";

fn main() {
    if let Err(err) = ensure_carrier_services() {
        eprintln!(
            "Carrier bootstrap skipped: unable to start local services automatically ({err})"
        );
        eprintln!("You can still open the app, but WebUI may show connection issues.");
    }

    tauri::Builder::default()
        .setup(|app| {
            if let Some(window) = app.get_webview_window("main") {
                if let Err(err) = window.eval(&format!(
                    "window.location.replace('{}');",
                    CARRIER_GATEWAY_URL
                )) {
                    eprintln!("Failed to navigate Tauri window to gateway: {err}");
                }
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running Tauri application");
}

fn ensure_carrier_services() -> Result<(), String> {
    if is_gateway_ready() {
        return Ok(());
    }

    if !command_available("carrier") {
        return Err("carrier executable is not available in PATH".into());
    }

    if !is_tcp_ready(CARRIER_DAEMON_ADDR, Duration::from_millis(150)) {
        spawn_background(&["daemon"])?;
        wait_for_tcp_ready(CARRIER_DAEMON_ADDR, Duration::from_secs(20))
            .map_err(|_| "daemon service did not become ready within timeout".to_string())?;
    }

    if !is_gateway_ready() {
        spawn_background(&["gateway"])?;
        wait_for_http_ready(CARRIER_GATEWAY_URL, Duration::from_secs(20))
            .map_err(|_| "gateway service did not become ready within timeout".to_string())?;
    }

    if is_gateway_ready() {
        return Ok(());
    }

    Err("gateway service is still not reachable after bootstrap".into())
}

fn command_available(cmd: &str) -> bool {
    Command::new(cmd)
        .arg("--version")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .output()
        .map(|out| out.status.success())
        .unwrap_or(false)
}

fn spawn_background(args: &[&str]) -> Result<(), String> {
    let mut command = Command::new("carrier");
    command.args(args);
    command.stdin(Stdio::null());
    command.stdout(Stdio::null());
    command.stderr(Stdio::null());
    command
        .spawn()
        .map_err(|err| format!("failed to spawn `carrier {}`: {err}", args.join(" ")))?;
    Ok(())
}

fn socket_addr(raw: &str) -> Result<SocketAddr, String> {
    SocketAddr::from_str(raw).map_err(|err| format!("invalid socket address {raw}: {err}"))
}

fn is_tcp_ready(raw_addr: &str, timeout: Duration) -> bool {
    socket_addr(raw_addr)
        .and_then(|addr| {
            let deadline = Instant::now() + timeout;
            while Instant::now() < deadline {
                if TcpStream::connect_timeout(&addr, Duration::from_millis(150)).is_ok() {
                    return Ok(());
                }
                sleep(Duration::from_millis(150));
            }
            Err("timeout")
        })
        .is_ok()
}

fn wait_for_tcp_ready(raw_addr: &str, timeout: Duration) -> Result<(), String> {
    if is_tcp_ready(raw_addr, timeout) {
        return Ok(());
    }
    Err(format!("timeout waiting for TCP {raw_addr}"))
}

fn wait_for_http_ready(url: &str, timeout: Duration) -> Result<(), String> {
    let deadline = Instant::now() + timeout;
    while Instant::now() < deadline {
        if is_http_ready(url) {
            return Ok(());
        }
        sleep(Duration::from_millis(250));
    }
    Err(format!("timeout waiting for HTTP {url}"))
}

fn is_http_ready(url: &str) -> bool {
    let host_addr = CARRIER_GATEWAY_ADDR;
    let addr = match socket_addr(host_addr) {
        Ok(addr) => addr,
        Err(_) => return false,
    };

    let mut stream = match TcpStream::connect_timeout(&addr, Duration::from_millis(150)) {
        Ok(s) => s,
        Err(_) => return false,
    };

    let request = format!(
        "GET {} HTTP/1.1\r\nHost: {}\r\nConnection: close\r\nUser-Agent: carrier-tauri-bootstrap\r\n\r\n",
        gateway_path(url),
        host_addr
    );
    if stream.write_all(request.as_bytes()).is_err() {
        return false;
    }

    let mut buffer = [0_u8; 1024];
    let read_len = stream.read(&mut buffer).unwrap_or(0);
    if read_len == 0 {
        return false;
    }
    let text = String::from_utf8_lossy(&buffer[..read_len]);
    let mut lines = text.split_whitespace();
    let _http_version = lines.next();
    let status = lines.next().and_then(|status| status.parse::<u16>().ok());
    matches!(status, Some(code) if (200..400).contains(&code))
}

fn gateway_path(url: &str) -> &str {
    match url.find("://") {
        Some(scheme_pos) => {
            let path_part = &url[(scheme_pos + 3)..];
            let slash_pos = path_part.find('/').unwrap_or(path_part.len());
            if slash_pos + (scheme_pos + 3) == url.len() {
                return "/";
            }
            &url[(scheme_pos + 3 + slash_pos)..]
        }
        None => "/",
    }
}

fn is_gateway_ready() -> bool {
    is_http_ready(CARRIER_GATEWAY_URL)
}
