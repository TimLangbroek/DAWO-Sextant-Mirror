//! `sextant-agent join <console-url> <code> [--name NAME]`: exchange a code
//! from the console for this laptop's tag and credential (design 0015).
//!
//! Run once, as root, by a person or by the installer. It says who the
//! machine is - serial, vendor, model, processor, whether it is virtual -
//! and writes what comes back where the agent and comin read it:
//!
//!   <state>/credential    the device credential, 0600, no trailing newline
//!                         (the agent, and comin to pull the fleet repository)
//!   <state>/joined.json   tag, console address and branch (the agent, when
//!                         its unit does not set them; config.rs)
//!
//! The console decides everything else (design 0015): which device this
//! is, its group and its hardware profile. Every refusal arrives in words
//! for the person at the laptop and is printed as it is.

use serde::{Deserialize, Serialize};
use std::fs;
use std::io::Write;
use std::os::unix::fs::OpenOptionsExt;
use std::path::{Path, PathBuf};
use std::process::{Command, ExitCode};
use std::time::Duration;

/// Where the agent keeps its state on an installed laptop. The installer
/// passes the target's own (`--state-dir /mnt/var/lib/sextant-agent`).
pub const STATE_DIR: &str = "/var/lib/sextant-agent";

/// How long the console may take to answer: it waits for the gate, and a
/// cold remote gate takes minutes. The console allows the same (JoinWait,
/// internal/http/api/join.go).
const WAIT: Duration = Duration::from_secs(15 * 60);

const USAGE: &str =
    "usage: sextant-agent join <console-url> <code> [--name NAME] [--state-dir DIR]";

#[derive(Debug, PartialEq)]
pub struct Args {
    pub url: String,
    pub code: String,
    pub name: String,
    pub state_dir: PathBuf,
}

/// parse reads the arguments after `join`.
pub fn parse(args: &[String]) -> Result<Args, String> {
    let mut positional = Vec::new();
    let mut name = String::new();
    let mut state_dir = PathBuf::from(STATE_DIR);
    let mut it = args.iter();
    while let Some(a) = it.next() {
        let mut value = |flag: &str| -> Result<String, String> {
            it.next()
                .cloned()
                .ok_or_else(|| format!("{flag} needs a value\n{USAGE}"))
        };
        match a.as_str() {
            "--name" => name = value("--name")?,
            "--state-dir" => state_dir = PathBuf::from(value("--state-dir")?),
            s if s.starts_with("--name=") => name = s["--name=".len()..].to_string(),
            s if s.starts_with("--state-dir=") => {
                state_dir = PathBuf::from(&s["--state-dir=".len()..])
            }
            s if s.starts_with("--") => return Err(format!("unknown option {s}\n{USAGE}")),
            _ => positional.push(a.clone()),
        }
    }
    let [url, code] = positional.as_slice() else {
        return Err(USAGE.to_string());
    };
    let url = url.trim_end_matches('/').to_string();
    if !url.starts_with("https://") && !crate::config::is_loopback_http(&url) {
        return Err(format!(
            "the console address must start with https:// (got {url})"
        ));
    }
    Ok(Args {
        url,
        code: code.clone(),
        name,
        state_dir,
    })
}

/// Machine is what the laptop says about itself.
#[derive(Debug, Default, PartialEq, Serialize)]
pub struct Machine {
    pub serial: String,
    pub vendor: String,
    pub model: String,
    pub cpu: String,
    pub virtualisation: String,
}

/// machine reads the firmware's identity under root ("/" on a real
/// machine). The serial needs root to read; without it the console names
/// the laptop at random and cannot tell it apart from a second join.
pub fn machine(root: &Path) -> Machine {
    let read = |p: &str| {
        fs::read_to_string(root.join(p))
            .map(|s| s.trim().to_string())
            .unwrap_or_default()
    };
    let cpu = read("proc/cpuinfo")
        .lines()
        .find_map(|l| {
            let (k, v) = l.split_once(':')?;
            (k.trim() == "vendor_id").then(|| v.trim().to_string())
        })
        .unwrap_or_default();
    Machine {
        serial: read("sys/class/dmi/id/product_serial"),
        vendor: read("sys/class/dmi/id/sys_vendor"),
        model: read("sys/class/dmi/id/product_name"),
        cpu,
        virtualisation: String::new(),
    }
}

/// detect_virt asks systemd, which prints "none" on real hardware (and
/// exits 1 doing so, which is not a failure here).
fn detect_virt() -> String {
    Command::new("systemd-detect-virt")
        .output()
        .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
        .unwrap_or_default()
}

#[derive(Serialize)]
struct Request<'a> {
    code: &'a str,
    #[serde(skip_serializing_if = "str::is_empty")]
    name: &'a str,
    #[serde(flatten)]
    machine: &'a Machine,
}

#[derive(Debug, Deserialize, PartialEq)]
pub struct Answer {
    pub tag: String,
    pub credential: String,
    pub console: Console,
}

#[derive(Debug, Deserialize, PartialEq, Serialize)]
pub struct Console {
    pub url: String,
    pub branch: String,
}

#[derive(Debug, PartialEq)]
pub enum Failure {
    /// The console answered and said no. Final: trying again changes nothing.
    Refused(String),
    /// The console did not answer, or not in a way that settles it.
    Transient(String),
}

/// exchange presents the code once.
pub fn exchange(url: &str, code: &str, name: &str, m: &Machine) -> Result<Answer, Failure> {
    let agent = ureq::AgentBuilder::new()
        .timeout_connect(Duration::from_secs(15))
        .timeout_read(WAIT)
        .build();
    let body = Request {
        code,
        name,
        machine: m,
    };
    match agent.post(&format!("{url}/api/join")).send_json(&body) {
        Ok(resp) => resp.into_json::<Answer>().map_err(|e| {
            Failure::Transient(format!("the console's answer could not be read: {e}"))
        }),
        Err(ureq::Error::Status(status, resp)) => {
            let said = resp
                .into_json::<serde_json::Value>()
                .ok()
                .and_then(|v| v.get("error")?.as_str().map(str::to_string))
                .unwrap_or_else(|| format!("the console answered {status}"));
            // 4xx is the console's decision. 5xx, and a proxy's 502/504 in
            // front of a slow gate, may well go through on a second try: the
            // console gives a use back to a laptop that left early.
            if (400..500).contains(&status) {
                Err(Failure::Refused(said))
            } else {
                Err(Failure::Transient(said))
            }
        }
        Err(e) => Err(Failure::Transient(format!(
            "the console could not be reached: {e}"
        ))),
    }
}

#[derive(Serialize)]
struct Joined<'a> {
    tag: &'a str,
    url: &'a str,
    branch: &'a str,
}

/// write_state stores the answer: the credential first, atomically and
/// readable by root only, then joined.json. A credential written with a
/// trailing newline is a classic way to a 401 that nobody can see, so it is
/// written exactly as received.
pub fn write_state(dir: &Path, a: &Answer, console_url: &str) -> std::io::Result<()> {
    fs::create_dir_all(dir)?;
    write_atomic(dir, "credential", a.credential.as_bytes(), 0o600)?;
    let url = if a.console.url.is_empty() {
        console_url
    } else {
        &a.console.url
    };
    let joined = serde_json::to_vec_pretty(&Joined {
        tag: &a.tag,
        url,
        branch: &a.console.branch,
    })?;
    write_atomic(dir, "joined.json", &joined, 0o644)
}

fn write_atomic(dir: &Path, name: &str, data: &[u8], mode: u32) -> std::io::Result<()> {
    let tmp = dir.join(format!(".{name}.tmp"));
    let _ = fs::remove_file(&tmp);
    let mut f = fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(mode)
        .open(&tmp)?;
    f.write_all(data)?;
    f.sync_all()?;
    fs::rename(&tmp, dir.join(name))
}

/// run is the subcommand. Exit codes: 0 joined, 2 usage, 4 refused by the
/// console, 5 the console could not be reached, 6 the answer could not be
/// written.
pub fn run(args: &[String]) -> ExitCode {
    let args = match parse(args) {
        Ok(a) => a,
        Err(e) => {
            eprintln!("{e}");
            return ExitCode::from(2);
        }
    };
    let mut m = machine(Path::new("/"));
    m.virtualisation = detect_virt();
    if m.serial.is_empty() {
        eprintln!(
            "note: no serial number readable (run as root); the console will pick a random name"
        );
    }
    println!(
        "Joining {} as {} {} (serial {}). The console checks the new configuration first; this can take a few minutes.",
        args.url,
        m.vendor,
        m.model,
        if m.serial.is_empty() { "unknown" } else { &m.serial }
    );
    let mut answer = None;
    for (attempt, pause) in [0u64, 10, 30, 60].iter().enumerate() {
        if *pause > 0 {
            std::thread::sleep(Duration::from_secs(*pause));
        }
        match exchange(&args.url, &args.code, &args.name, &m) {
            Ok(a) => {
                answer = Some(a);
                break;
            }
            Err(Failure::Refused(why)) => {
                eprintln!("The console refused: {why}");
                return ExitCode::from(4);
            }
            Err(Failure::Transient(why)) if attempt < 3 => {
                eprintln!("{why}; trying again in {}s", [10, 30, 60][attempt]);
            }
            Err(Failure::Transient(why)) => {
                eprintln!("{why}. Giving up; the code still works, so run this again later.");
                return ExitCode::from(5);
            }
        }
    }
    let Some(answer) = answer else {
        return ExitCode::from(5);
    };
    if let Err(e) = write_state(&args.state_dir, &answer, &args.url) {
        eprintln!(
            "Joined as {}, but the credential could not be written to {}: {e}. Ask for a Re-enrol code for {} in the console.",
            answer.tag,
            args.state_dir.display(),
            answer.tag
        );
        return ExitCode::from(6);
    }
    println!(
        "Joined as {}. The credential is in {}.",
        answer.tag,
        args.state_dir.join("credential").display()
    );
    ExitCode::SUCCESS
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::{BufRead, BufReader, Read};
    use std::net::TcpListener;
    use std::os::unix::fs::PermissionsExt;
    use std::thread;

    fn s(v: &[&str]) -> Vec<String> {
        v.iter().map(|x| x.to_string()).collect()
    }

    #[test]
    fn parse_takes_the_address_the_code_and_two_options() {
        let a = parse(&s(&["https://console.example.org/", "K7QM-4XWD"])).unwrap();
        assert_eq!(
            a,
            Args {
                url: "https://console.example.org".into(),
                code: "K7QM-4XWD".into(),
                name: String::new(),
                state_dir: PathBuf::from(STATE_DIR),
            }
        );
        let a = parse(&s(&[
            "--name",
            "lt-9",
            "https://c.example",
            "--state-dir=/mnt/x",
            "CODE",
        ]))
        .unwrap();
        assert_eq!(
            (a.name.as_str(), a.state_dir.as_path(), a.code.as_str()),
            ("lt-9", Path::new("/mnt/x"), "CODE")
        );
        let a = parse(&s(&["http://127.0.0.1:8080", "CODE", "--name=lt-1"])).unwrap();
        assert_eq!(a.name, "lt-1");

        for bad in [
            &["https://c.example"][..],
            &["https://c.example", "A", "B"],
            &["http://console.example.org", "CODE"],
            &["https://c.example", "CODE", "--name"],
            &["https://c.example", "CODE", "--force"],
        ] {
            assert!(parse(&s(bad)).is_err(), "{bad:?} parsed");
        }
    }

    #[test]
    fn machine_reads_the_firmware_and_the_processor() {
        let root = std::env::temp_dir().join(format!("sx-join-{}", std::process::id()));
        let dmi = root.join("sys/class/dmi/id");
        fs::create_dir_all(&dmi).unwrap();
        fs::create_dir_all(root.join("proc")).unwrap();
        fs::write(dmi.join("product_serial"), "PF3ABCDE\n").unwrap();
        fs::write(dmi.join("sys_vendor"), "LENOVO\n").unwrap();
        fs::write(dmi.join("product_name"), "21BRCTO1WW\n").unwrap();
        fs::write(
            root.join("proc/cpuinfo"),
            "processor\t: 0\nvendor_id\t: GenuineIntel\ncpu family\t: 6\nprocessor\t: 1\nvendor_id\t: GenuineIntel\n",
        )
        .unwrap();
        let m = machine(&root);
        fs::remove_dir_all(&root).unwrap();
        assert_eq!(
            m,
            Machine {
                serial: "PF3ABCDE".into(),
                vendor: "LENOVO".into(),
                model: "21BRCTO1WW".into(),
                cpu: "GenuineIntel".into(),
                virtualisation: String::new(),
            }
        );
        // Nothing readable is not an error: the console decides what it means.
        assert_eq!(machine(Path::new("/nonexistent-root")), Machine::default());
    }

    #[test]
    fn write_state_keeps_the_credential_exact_and_private() {
        let dir = std::env::temp_dir().join(format!("sx-state-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        let a = Answer {
            tag: "laptop-3abcde".into(),
            credential: "sxt_device-laptop-3abcde_secret".into(),
            console: Console {
                url: String::new(),
                branch: "stable".into(),
            },
        };
        write_state(&dir, &a, "https://typed.example").unwrap();
        // A second join replaces the first.
        let b = Answer {
            credential: "sxt_device-laptop-3abcde_rotated".into(),
            ..a
        };
        write_state(&dir, &b, "https://typed.example").unwrap();
        let cred = fs::read(dir.join("credential")).unwrap();
        let mode = fs::metadata(dir.join("credential"))
            .unwrap()
            .permissions()
            .mode()
            & 0o777;
        let joined: serde_json::Value =
            serde_json::from_slice(&fs::read(dir.join("joined.json")).unwrap()).unwrap();
        let leftovers: Vec<_> = fs::read_dir(&dir)
            .unwrap()
            .map(|e| e.unwrap().file_name().into_string().unwrap())
            .filter(|n| n.starts_with('.'))
            .collect();
        fs::remove_dir_all(&dir).unwrap();
        assert_eq!(cred, b"sxt_device-laptop-3abcde_rotated");
        assert_eq!(mode, 0o600);
        assert_eq!(joined["tag"], "laptop-3abcde");
        // The console did not say where it is, so the typed address stands.
        assert_eq!(joined["url"], "https://typed.example");
        assert_eq!(joined["branch"], "stable");
        assert!(leftovers.is_empty(), "temporary files left: {leftovers:?}");
    }

    /// answer serves one request with a canned status and JSON body, and
    /// returns the request body it got.
    fn answer(status: u16, body: &'static str) -> (String, thread::JoinHandle<String>) {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let addr = listener.local_addr().unwrap();
        let h = thread::spawn(move || {
            let (mut stream, _) = listener.accept().unwrap();
            let mut reader = BufReader::new(stream.try_clone().unwrap());
            let (mut line, mut len, mut path) = (String::new(), 0usize, String::new());
            loop {
                line.clear();
                reader.read_line(&mut line).unwrap();
                if path.is_empty() {
                    path = line.split(' ').nth(1).unwrap_or("").to_string();
                }
                if let Some(v) = line.to_ascii_lowercase().strip_prefix("content-length:") {
                    len = v.trim().parse().unwrap();
                }
                if line == "\r\n" {
                    break;
                }
            }
            let mut req = vec![0u8; len];
            reader.read_exact(&mut req).unwrap();
            write!(
                stream,
                "HTTP/1.1 {status} X\r\ncontent-type: application/json\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
                body.len()
            )
            .unwrap();
            format!("{path} {}", String::from_utf8_lossy(&req))
        });
        (format!("http://127.0.0.1:{}", addr.port()), h)
    }

    fn lenovo() -> Machine {
        Machine {
            serial: "PF3ABCDE".into(),
            vendor: "LENOVO".into(),
            model: "21BR".into(),
            cpu: "GenuineIntel".into(),
            virtualisation: "none".into(),
        }
    }

    #[test]
    fn exchange_sends_the_machine_and_reads_the_answer() {
        let (url, h) = answer(
            201,
            r#"{"tag":"laptop-3abcde","credential":"sxt_x","console":{"url":"https://c.example","branch":"main"}}"#,
        );
        let got = exchange(&url, "K7QM-4XWD", "", &lenovo()).unwrap();
        assert_eq!(got.tag, "laptop-3abcde");
        assert_eq!(got.credential, "sxt_x");
        let req = h.join().unwrap();
        let (path, body) = req.split_once(' ').unwrap();
        assert_eq!(path, "/api/join");
        let body: serde_json::Value = serde_json::from_str(body).unwrap();
        assert_eq!(
            body,
            serde_json::json!({"code":"K7QM-4XWD","serial":"PF3ABCDE","vendor":"LENOVO",
                "model":"21BR","cpu":"GenuineIntel","virtualisation":"none"})
        );
    }

    #[test]
    fn exchange_tells_a_refusal_from_a_hiccup() {
        let (url, _) = answer(403, r#"{"error":"this code does not work"}"#);
        assert_eq!(
            exchange(&url, "X", "", &lenovo()),
            Err(Failure::Refused("this code does not work".into()))
        );
        let (url, _) = answer(409, r#"{"error":"already enrolled as lt-1"}"#);
        assert_eq!(
            exchange(&url, "X", "lt-9", &lenovo()),
            Err(Failure::Refused("already enrolled as lt-1".into()))
        );
        let (url, _) = answer(504, "<html>gateway timeout</html>");
        assert_eq!(
            exchange(&url, "X", "", &lenovo()),
            Err(Failure::Transient("the console answered 504".into()))
        );
        let (url, _) = answer(500, r#"{"error":"could not finish the join"}"#);
        assert_eq!(
            exchange(&url, "X", "", &lenovo()),
            Err(Failure::Transient("could not finish the join".into()))
        );
        // Nobody listening.
        let port = TcpListener::bind("127.0.0.1:0")
            .unwrap()
            .local_addr()
            .unwrap()
            .port();
        assert!(matches!(
            exchange(&format!("http://127.0.0.1:{port}"), "X", "", &lenovo()),
            Err(Failure::Transient(_))
        ));
    }
}
