//! burrow-forward relays TCP connections from one listening address to one fixed target.
//!
//! It publishes a port of a sandbox that is already running (R9): Podman cannot add a published
//! port to a running container, so burrow starts this forwarder next to it. The target is fixed at
//! start and must be an IP:port; the forwarder never dials anything else, so it is not a way out
//! for the sandbox.
//!
//! Standard library only (no crates), so there is no third-party code to pin or audit.
#![forbid(unsafe_code)]

use std::io::{self, Write};
use std::net::{Shutdown, SocketAddr, TcpListener, TcpStream};
use std::process::ExitCode;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;
use std::thread;
use std::time::Duration;

/// Time allowed to connect to the target.
const DIAL_TIMEOUT: Duration = Duration::from_secs(5);
/// Connections relayed at the same time; more are refused, so a flood cannot exhaust threads.
const MAX_CONNS: usize = 256;

#[derive(Debug, PartialEq)]
struct Config {
    listen: SocketAddr,
    target: SocketAddr,
}

/// Parses `-listen ADDR -target ADDR` (one or two dashes, as Go's flag package accepts).
fn parse_args<I: IntoIterator<Item = String>>(args: I) -> Result<Config, String> {
    let (mut listen, mut target) = (None, None);
    let mut it = args.into_iter();
    while let Some(a) = it.next() {
        let name = a.trim_start_matches('-');
        if name.len() == a.len() {
            return Err(format!("unexpected argument {a:?}"));
        }
        let (key, inline) = match name.split_once('=') {
            Some((k, v)) => (k.to_string(), Some(v.to_string())),
            None => (name.to_string(), None),
        };
        let value = match inline {
            Some(v) => v,
            None => it.next().ok_or_else(|| format!("-{key} needs a value"))?,
        };
        let addr: SocketAddr = value
            .parse()
            .map_err(|_| format!("-{key} {value:?}: want IP:port"))?;
        match key.as_str() {
            "listen" => listen = Some(addr),
            "target" => target = Some(addr),
            _ => return Err(format!("unknown flag -{key}")),
        }
    }
    match (listen, target) {
        (Some(listen), Some(target)) => Ok(Config { listen, target }),
        _ => Err("both -listen and -target are required".into()),
    }
}

/// Writes one JSON log line to stderr. Values are escaped; nothing secret is ever logged.
fn log(level: &str, msg: &str, fields: &[(&str, &str)]) {
    let mut line = format!("{{\"level\":\"{level}\",\"msg\":{}", json(msg));
    for (k, v) in fields {
        line.push_str(&format!(",\"{k}\":{}", json(v)));
    }
    line.push_str("}\n");
    let _ = io::stderr().write_all(line.as_bytes()); // nowhere else to report a failed log write
}

fn json(s: &str) -> String {
    let mut out = String::from("\"");
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out.push('"');
    out
}

/// Copies both ways until either side closes, then closes both.
fn pipe(a: TcpStream, b: TcpStream) -> io::Result<()> {
    let (a2, b2) = (a.try_clone()?, b.try_clone()?);
    let one = thread::spawn(move || copy_then_close(a2, b2));
    copy_then_close(b, a);
    let _ = one.join(); // the copy thread only ends; it has no result to report
    Ok(())
}

fn copy_then_close(mut src: TcpStream, mut dst: TcpStream) {
    let _ = io::copy(&mut src, &mut dst); // a reset is the normal end of a relayed connection
    let _ = dst.shutdown(Shutdown::Write);
    let _ = src.shutdown(Shutdown::Read);
}

/// Accepts on `ln` and relays each connection to `target`, forever.
fn serve(ln: TcpListener, target: SocketAddr) -> io::Result<()> {
    let active = Arc::new(AtomicUsize::new(0));
    for conn in ln.incoming() {
        let client = match conn {
            Ok(c) => c,
            Err(e) => {
                log("warn", "accept failed", &[("err", &e.to_string())]);
                continue;
            }
        };
        if active.fetch_add(1, Ordering::SeqCst) >= MAX_CONNS {
            active.fetch_sub(1, Ordering::SeqCst);
            log("warn", "too many connections, refused", &[]);
            continue; // dropping the stream closes it
        }
        let active = Arc::clone(&active);
        thread::spawn(move || {
            let peer = client
                .peer_addr()
                .map(|a| a.to_string())
                .unwrap_or_default();
            match TcpStream::connect_timeout(&target, DIAL_TIMEOUT) {
                Ok(up) => {
                    if let Err(e) = pipe(client, up) {
                        log(
                            "warn",
                            "relay failed",
                            &[("client", &peer), ("err", &e.to_string())],
                        );
                    }
                }
                Err(e) => log(
                    "warn",
                    "dial failed",
                    &[
                        ("client", &peer),
                        ("target", &target.to_string()),
                        ("err", &e.to_string()),
                    ],
                ),
            }
            active.fetch_sub(1, Ordering::SeqCst);
        });
    }
    Ok(())
}

fn main() -> ExitCode {
    let cfg = match parse_args(std::env::args().skip(1)) {
        Ok(c) => c,
        Err(e) => {
            log("error", &e, &[]);
            return ExitCode::from(2);
        }
    };
    let ln = match TcpListener::bind(cfg.listen) {
        Ok(l) => l,
        Err(e) => {
            log("error", "listen", &[("err", &e.to_string())]);
            return ExitCode::from(1);
        }
    };
    log(
        "info",
        "forwarding",
        &[
            ("listen", &cfg.listen.to_string()),
            ("target", &cfg.target.to_string()),
        ],
    );
    match serve(ln, cfg.target) {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            log("error", "serve", &[("err", &e.to_string())]);
            ExitCode::from(1)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Read;

    fn args(s: &str) -> Vec<String> {
        s.split_whitespace().map(String::from).collect()
    }

    #[test]
    fn parses_go_style_flags() {
        let want = Config {
            listen: "0.0.0.0:8080".parse().unwrap(),
            target: "10.89.0.2:8080".parse().unwrap(),
        };
        assert_eq!(
            parse_args(args("-listen 0.0.0.0:8080 -target 10.89.0.2:8080")),
            Ok(want)
        );
        let want2 = Config {
            listen: "0.0.0.0:1".parse().unwrap(),
            target: "10.0.0.1:2".parse().unwrap(),
        };
        assert_eq!(
            parse_args(args("--listen=0.0.0.0:1 --target=10.0.0.1:2")),
            Ok(want2)
        );
    }

    #[test]
    fn refuses_anything_but_ip_port() {
        for bad in [
            "-listen 0.0.0.0:8080",                         // no target
            "-listen 0.0.0.0:8080 -target example.com:443", // a name could be re-resolved
            "-listen 0.0.0.0:8080 -target 10.0.0.1",        // no port
            "-listen 0.0.0.0:8080 -target 10.0.0.1:2 -x 1", // unknown flag
            "-listen 0.0.0.0:8080 -target 10.0.0.1:2 extra",
            "-target",
        ] {
            assert!(parse_args(args(bad)).is_err(), "{bad}");
        }
    }

    #[test]
    fn json_escapes() {
        assert_eq!(json("a\"b\\c\nd"), "\"a\\\"b\\\\c\\u000ad\"");
    }

    #[test]
    fn relays_both_ways() {
        let echo = TcpListener::bind("127.0.0.1:0").unwrap();
        let target = echo.local_addr().unwrap();
        thread::spawn(move || {
            for c in echo.incoming().flatten() {
                thread::spawn(move || {
                    let mut r = c.try_clone().unwrap();
                    let mut w = c;
                    let _ = io::copy(&mut r, &mut w);
                });
            }
        });
        let ln = TcpListener::bind("127.0.0.1:0").unwrap();
        let addr = ln.local_addr().unwrap();
        thread::spawn(move || serve(ln, target));

        let mut c = TcpStream::connect(addr).unwrap();
        c.write_all(b"ping").unwrap();
        c.shutdown(Shutdown::Write).unwrap();
        let mut got = String::new();
        c.read_to_string(&mut got).unwrap();
        assert_eq!(got, "ping");
    }
}
