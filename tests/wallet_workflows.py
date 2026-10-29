#!/usr/bin/env python3
"""Exercise private wallet custody and metadata through actual CLI processes."""
import argparse
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="bl-wallet-workflows-") as temporary:
        work = Path(temporary)
        binary = args.binary.resolve() if args.binary else work / "blwallet"
        if not args.binary:
            env = dict(os.environ, GOCACHE=str(ROOT / ".cache/go-build"),
                       GOMODCACHE=str(ROOT / ".cache/go-mod"))
            subprocess.run(["go", "build", "-o", str(binary), "./cmd/blwallet"],
                           cwd=ROOT, env=env, check=True)
        old, new = work / "old-password", work / "new-password"
        for path in (old, new):
            path.write_text(secrets.token_hex(24) + "\n")
            path.chmod(0o600)

        def run(*arguments, success=True):
            result = subprocess.run([str(binary), *map(str, arguments)],
                                    capture_output=True, text=True, timeout=30)
            if (result.returncode == 0) != success:
                raise RuntimeError(f"unexpected wallet exit: {arguments[0]}: {result.stderr}")
            return json.loads(result.stdout) if success else None

        wallet, restored, backup = work / "wallet", work / "restored", work / "backup"
        account = run("create", "--wallet", wallet, "--password-file", old)["account"]
        run("names", "--wallet", wallet, "--action", "add", "--name", "Savings", "--index", 9)
        named = run("accounts", "--wallet", wallet, "--password-file", old, "--account-name", "Savings")[0]
        if named["index"] != 9:
            raise RuntimeError("named account index changed")
        run("contacts", "--wallet", wallet, "--action", "add", "--name", "Recipient",
            "--address", account, "--network", "0" * 64)
        contacts = work / "contacts"
        run("contacts", "--wallet", wallet, "--action", "export", "--file", contacts)
        run("backup", "--wallet", wallet, "--password-file", old, "--out", backup)
        run("restore", "--wallet", restored, "--password-file", old, "--backup", backup)
        recovered = run("accounts", "--wallet", restored, "--password-file", old, "--index", 9)[0]
        if recovered != named:
            raise RuntimeError("restored wallet changed deterministic identity")
        run("contacts", "--wallet", restored, "--action", "import", "--file", contacts)
        if run("contacts", "--wallet", wallet) != run("contacts", "--wallet", restored):
            raise RuntimeError("contact archive changed recipients")
        run("password", "--wallet", wallet, "--password-file", old, "--new-password-file", new)
        run("accounts", "--wallet", wallet, "--password-file", old, success=False)
        if run("accounts", "--wallet", wallet, "--password-file", new)[0]["account"] != account:
            raise RuntimeError("password rotation changed identity")
        run("outbox", "--wallet", wallet, success=False)
        profile = Path(str(wallet) + ".profile.json")
        profile.chmod(0o644)
        run("contacts", "--wallet", wallet, success=False)
    print(json.dumps({"status": "passed", "workflows": ["custody", "account_names", "contacts",
                                                            "backup_restore", "password_rotation",
                                                            "private_profile", "missing_outbox"]}))


if __name__ == "__main__":
    main()
