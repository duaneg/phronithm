fn main() {
    let a: Vec<String> = std::env::args().collect();
    println!("{:?}", probe::call(&a[1], &[("STUB_MODE", "ok")], std::time::Duration::from_secs(10)));
}
