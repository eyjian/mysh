#!/usr/bin/env python3
"""Create GitHub Release and upload prebuilt binaries.

Usage:
    GITHUB_TOKEN=xxx python3 scripts/release.py [version]

If version is not specified, uses the latest git tag.
Requires: pip install requests
"""
import os, sys, subprocess, requests

REPO = "eyjian/mysh"

def get_version():
    if len(sys.argv) > 1:
        return sys.argv[1]
    result = subprocess.run(
        ["git", "describe", "--tags", "--abbrev=0"],
        capture_output=True, text=True,
    )
    if result.returncode != 0:
        print("ERROR: No git tags found. Specify version as argument.")
        sys.exit(1)
    return result.stdout.strip()

def main():
    token = os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN")
    if not token:
        print("ERROR: Set GITHUB_TOKEN or GH_TOKEN environment variable")
        sys.exit(1)

    version = get_version()
    if not version.startswith("v"):
        version = f"v{version}"

    print(f"Creating release {version} for {REPO}...")

    api = f"https://api.github.com/repos/{REPO}"
    headers = {"Authorization": f"token {token}"}

    # Check if release already exists
    resp = requests.get(f"{api}/releases/tags/{version}", headers=headers)
    if resp.status_code == 200:
        release = resp.json()
        print(f"Release already exists (id={release['id']})")
    else:
        # Create release
        body = f"""## mysh {version}

### Prebuilt Binaries

| Platform | Arch | File |
|----------|------|------|
| Linux | amd64 | `mysh-linux-amd64` |
| Linux | arm64 | `mysh-linux-arm64` |
| macOS | amd64 | `mysh-darwin-amd64` |
| macOS | arm64 | `mysh-darwin-arm64` |
| Windows | amd64 | `mysh-windows-amd64.exe` |
| Windows | arm64 | `mysh-windows-arm64.exe` |

### Install

```bash
curl -sSL https://raw.githubusercontent.com/eyjian/mysh/main/install.sh | bash
```"""

        resp = requests.post(f"{api}/releases", headers=headers, json={
            "tag_name": version,
            "name": version,
            "body": body,
            "draft": False,
            "prerelease": False,
        })
        if resp.status_code not in (200, 201):
            print(f"Failed to create release: {resp.status_code} {resp.text}")
            sys.exit(1)
        release = resp.json()
        print(f"Release created (id={release['id']})")

    upload_url = release["upload_url"].split("{")[0]

    # Build binaries first
    print("Cross-compiling binaries...")
    result = subprocess.run(["make", "cross-compile"], cwd=os.path.dirname(__file__) + "/..")
    if result.returncode != 0:
        print("ERROR: Cross-compilation failed")
        sys.exit(1)

    # Upload assets
    files = [
        "mysh-linux-amd64", "mysh-linux-arm64",
        "mysh-darwin-amd64", "mysh-darwin-arm64",
        "mysh-windows-amd64.exe", "mysh-windows-arm64.exe",
    ]
    root = os.path.join(os.path.dirname(__file__), "..")

    for f in files:
        path = os.path.join(root, f)
        if not os.path.exists(path):
            print(f"  SKIP: {f} not found")
            continue
        print(f"  Uploading {f}...")
        with open(path, "rb") as fh:
            r = requests.post(
                upload_url,
                headers={**headers, "Content-Type": "application/octet-stream"},
                params={"name": f},
                data=fh,
            )
        if r.status_code in (200, 201):
            print(f"  OK: {f}")
        else:
            print(f"  FAIL: {r.status_code} {r.text[:200]}")

    print(f"\nRelease {version} published: https://github.com/{REPO}/releases/tag/{version}")

if __name__ == "__main__":
    main()
