# Windows (and macOS)

> **Share only a dedicated workspace folder with the Podman machine.** Whatever the hypervisor can
> see is exposed to the guest OS (the Linux VM) and to everything that runs there, sandboxes
> included if a mount or an escape reaches it. Never share folders that hold credentials, secrets,
> caches, personal files or other sensitive information: not your user folder (`C:\Users\me`),
> not `.ssh`, `.aws`, `.config`, `AppData`, browser profiles or package caches. Create a folder
> for work with agents, for example `C:\burrow-work`, put only the projects you want agents to
> see in it, and share only that.
>
> **The WSL backend is highly discouraged for now.** Nothing has been done yet to secure it, and
> by default it mounts every Windows drive into the VM (the whole host directory tree is visible),
> runs Windows programs from inside (interop), and lets the default user become root. Use the
> Hyper-V backend, which shares only the folders you name. WSL stays on the list only to be tested
> and hardened (exception E1, below).

> **Status on Windows (2026-11-22, from J's tests).** Neither setup works out of the box yet:
>
> - **Hyper-V and WSL Podman machines cannot build Go from a shared folder.** The share has no
>   file locking, and Go locks `go.mod` (`go: RLock /work/go.mod: function not implemented`).
>   To fix or work around (see "Known issues"); building burrow itself works with
>   `BUILD_MODE=context sh scripts/build-in-container.sh`, and the developer checks with
>   `podman build --target vet|lint|test|vuln -f packaging/dev/Dockerfile .`; neither uses the share.
> - **Podman inside a WSL distro** (not a machine) is where the work so far has been done, with the
>   repository under `/mnt/c/...`. Ubuntu 22.04's Podman 3.4.4 is too old: its CNI network
>   backend fails on burrow's networks (`plugin firewall does not support config version
>   "1.0.0"`). Use a distro with Podman 4.9+ and netavark (Ubuntu 24.04), and keep the repository
>   in the distro's own disk (`~/src`), not under `/mnt/c`. burrow now refuses older Podman or
>   CNI at start and says so.
>   J also needed this in `~/.config/containers/containers.conf` for Podman in WSL to work
>   (*why is unverified*; likely a distro without systemd, so no systemd cgroup manager):
>
>   ```toml
>   [engine]
>   cgroup_manager = "cgroupfs"
>   runtime = "crun"
>   ```
> - **Do not install mise, Go or other tools on the Windows host** to work around this: keeping
>   tools off the host is the point. Everything builds in pinned containers (README, "Try it").

**macOS:** discovery is pending. The Podman machine there is also a VM with shared folders, so the
same rule applies (share one work folder only). Trying it, finding the security issues and
recommending an install and usage path is on the [roadmap](ROADMAP.md) (0.2).

## How burrow runs on Windows

burrow runs on Windows as a normal program; `podman` is the remote client of a **Podman machine**
(a Linux VM) where the sandboxes run. Two backends, to be tested side by side (J, 2026-11-18):

| | Hyper-V backend (recommended) | WSL backend (discouraged; exception E1) |
|---|---|---|
| Windows editions | Pro, Enterprise, Education | all, Home included |
| Rights | *Hyper-V Administrators* (or admin) to create the machine | none beyond WSL |
| Isolation from the rest of Windows | a VM of its own | one VM and kernel shared with every WSL distro; Windows drives mounted unless turned off; interop runs Windows programs unless turned off; root available by default |
| Folders | only folders shared at `podman machine init -v` (9p) | every Windows drive is reachable through the WSL mounts |
| Ports | forwarded to `localhost` on Windows | published ports reach `localhost` on Windows and from other distros |

*From documentation, to verify on a real machine.*

## Try it

```powershell
# Once: a machine that shares ONLY the work folder (Hyper-V, needs admin or Hyper-V Administrators)
mkdir C:\burrow-work
podman machine init --provider hyperv -v C:\burrow-work:/mnt/burrow-work burrow-hv
podman machine start burrow-hv
podman system connection default burrow-hv

# In a clone of this repository (inside C:\burrow-work)
# Only Podman: burrow and its images are built in pinned containers (no Go, mise or make).
# burrow: the build script in the pinned Go image, this folder mounted (as burrow runs a tool)
podman run --rm --read-only --cap-drop ALL --security-opt no-new-privileges --http-proxy=false `
  --tmpfs /tmp:rw,exec,size=2g -e HOME=/tmp -e GOCACHE=/tmp/go-build -e GOMODCACHE=/tmp/gomod `
  -e CGO_ENABLED=0 -e "GOFLAGS=-trimpath -mod=readonly" -e GOTOOLCHAIN=local `
  -v "${PWD}:/src" -w /src docker.io/library/golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 sh scripts/build.sh
# On the Hyper-V share this fails with "RLock go.mod" (no file locking): build from the context:
#   podman build -f packaging/burrow/Dockerfile -t localhost/burrow-dist:dev .
#   $c = podman create localhost/burrow-dist:dev /none; podman cp "${c}:/dist" .\dist; podman rm $c
copy dist\burrow-windows-amd64.exe burrow.exe
podman build -f packaging/proxy/Dockerfile -t localhost/burrow-proxy:dev .
podman build -f packaging/forward/Dockerfile -t localhost/burrow-forward:dev .
foreach ($n in "alpine","curl","trivy") { podman build -t "localhost/burrow-test-${n}:dev" "test/images/$n" }
$env:BURROW_PRESETS_DIR = "$PWD\test\presets"
.\burrow.exe run alpine --name dev --workspace C:\burrow-work\project   # mounted at /work
.\burrow.exe shell dev -- id
.\burrow.exe up -f examples\burrow.sandbox    # set WORKSPACE="C:\burrow-work\project" first
.\burrow.exe --dry-run up -f examples\burrow.sandbox   # shows exactly what is sent to podman
```

For the WSL test only: `podman machine init --provider wsl burrow-wsl`, then apply the hardening
below before putting any real work in it.

## What to record for each backend (into TESTING.md)

1. `make e2e` from Git Bash against the machine, or the commands above by hand.
2. Workspace: a folder under `C:\burrow-work` mounted at `/work`; file owner and permissions
   inside; speed of reading many small files.
3. Exposure: from inside the machine (`podman machine ssh`), which Windows folders are visible?
   Only the shared one is the expected answer.
4. Ports: `burrow port dev 8080` reachable from a Windows browser at `http://127.0.0.1:8080`.
5. Visibility (WSL): from another WSL distro, can you see the sandbox's processes, files or ports?
6. Rights needed, and what an enterprise policy blocks.

## Known issues

**`plugin firewall does not support config version "1.0.0"` / `CNI network ... not found`.**
Podman older than 4.9 with the CNI backend (Ubuntu 22.04 ships 3.4.4). burrow checks this before
it creates anything (`podman info`: version and network backend). Fix: Podman 4.9+ with netavark;
on WSL, a new Ubuntu 24.04 distro: `wsl --install -d Ubuntu-24.04`, then `sudo apt install podman`.

**`go: RLock /work/go.mod: function not implemented` (and other tools that lock files).** The
folder a Podman machine shares from the host (9p with Hyper-V, virtiofs on macOS) does not
support `flock` locks, and Go locks `go.mod` before reading it, with no setting to skip it. This
is in the hypervisor's file sharing, not in burrow or the container, so a flag cannot switch it
on. *Which share supports which lock calls is unverified; to record per backend (step 2 above).*
Until burrow can keep the workspace in a volume inside the machine (roadmap 0.2):
- to build burrow itself: `BUILD_MODE=context sh scripts/build-in-container.sh` (DEVELOPMENT.md), which never uses the share;
- for your own projects: clone into the sandbox's home (`burrow shell dev -- git clone ... ~/src`)
  and build there, or build on the host.

Similar reports: [golang/go#37461](https://github.com/golang/go/issues/37461),
[podman discussion #22752](https://github.com/containers/podman/discussions/22752),
[lima#971](https://github.com/lima-vm/lima/issues/971).

## WSL hardening (E1, to build)

A machine image of our own: `/etc/wsl.conf` with `[automount] enabled=false`, `mountFsTab=false`,
`[interop] enabled=false`, `appendWindowsPath=false`; an unprivileged default user with no sudo
and root locked. Root inside could undo it, so the shared kernel and network stay a recorded risk.
