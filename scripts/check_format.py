import pathlib
import subprocess
import sys

goroot = subprocess.check_output(["go", "env", "GOROOT"], text=True).strip()
directories = subprocess.check_output(["go", "list", "-f", "{{.Dir}}", "./..."], text=True).splitlines()
result = subprocess.run([str(pathlib.Path(goroot) / "bin/gofmt"), "-l", *directories], capture_output=True, text=True, check=True)
if result.stdout.strip():
    print("Run make fmt:\n" + result.stdout, file=sys.stderr)
    sys.exit(1)
print("PASS: gofmt")
