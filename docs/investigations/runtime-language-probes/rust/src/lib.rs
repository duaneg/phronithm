use std::io::{BufRead, BufReader};
use std::os::unix::process::CommandExt;
use std::process::{Command, Stdio};
use std::sync::mpsc;
use std::thread;
use std::time::Duration;

#[derive(Debug, PartialEq)]
pub enum Outcome {
    Ok(serde_json::Map<String, serde_json::Value>),
    Null,
    Timeout,
}

pub fn call(binary: &str, env: &[(&str, &str)], timeout: Duration) -> std::io::Result<Outcome> {
    let mut child = Command::new(binary)
        .args(["-p", "x", "--output-format", "stream-json"])
        .envs(env.iter().copied())
        .stdout(Stdio::piped())
        .process_group(0)
        .spawn()?;
    let pid = child.id() as i32;
    let stdout = child.stdout.take().unwrap();
    let (tx, rx) = mpsc::channel();
    thread::spawn(move || {
        let mut res = Outcome::Null;
        for line in BufReader::new(stdout).lines().map_while(Result::ok) {
            if let Ok(ev) = serde_json::from_str::<serde_json::Value>(&line) {
                if ev["type"] == "result" {
                    if let Some(o) = ev["structured_output"].as_object() {
                        res = Outcome::Ok(o.clone());
                    }
                }
            }
        }
        let _ = tx.send(res);
    });
    match rx.recv_timeout(timeout) {
        Ok(res) => { child.wait()?; Ok(res) }
        Err(_) => {
            unsafe { libc::kill(-pid, libc::SIGKILL) };
            child.wait()?;
            Ok(Outcome::Timeout)
        }
    }
}

pub fn parallel(binary: &str, n: usize) -> Vec<Outcome> {
    thread::scope(|s| {
        let hs: Vec<_> = (0..n)
            .map(|_| s.spawn(|| call(binary, &[("STUB_MODE", "ok")], Duration::from_secs(10)).unwrap()))
            .collect();
        hs.into_iter().map(|h| h.join().unwrap()).collect()
    })
}
