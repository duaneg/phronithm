use probe::*;
use std::time::{Duration, Instant};

const STUB: &str = concat!(env!("CARGO_MANIFEST_DIR"), "/../claude");

#[test]
fn ok() {
    match call(STUB, &[("STUB_MODE", "ok")], Duration::from_secs(10)).unwrap() {
        Outcome::Ok(o) => assert_eq!(o["n"], 3),
        other => panic!("{other:?}"),
    }
}
#[test]
fn null() {
    assert_eq!(call(STUB, &[("STUB_MODE", "nullout")], Duration::from_secs(10)).unwrap(), Outcome::Null);
}
#[test]
fn hang_killed() {
    let f = std::env::temp_dir().join(format!("rs-probe-{}", std::process::id()));
    let t = Instant::now();
    let o = call(STUB, &[("STUB_MODE", "hang"), ("STUB_PIDFILE", f.to_str().unwrap())], Duration::from_secs(1)).unwrap();
    assert_eq!(o, Outcome::Timeout);
    assert!(t.elapsed() < Duration::from_secs(3));
    std::thread::sleep(Duration::from_millis(100));
    let pid: i32 = std::fs::read_to_string(&f).unwrap().trim().parse().unwrap();
    let _ = std::fs::remove_file(&f);
    assert_ne!(unsafe { libc::kill(pid, 0) }, 0, "grandchild alive");
}
#[test]
fn par() {
    assert!(parallel(STUB, 8).iter().all(|o| matches!(o, Outcome::Ok(_))));
}
