# Windows (and macOS)

> **Share only a dedicated workspace folder with the Podman machine.** Whatever the hypervisor can
> see is exposed to the guest OS (the Linux VM) and to everything that runs there, sandboxes
> included if a mount or an escape reaches it. Never share folders that hold credentials, secrets,
> caches, personal files or other sensitive information: not your user folder (`C:\Users\me`),
> not `.ssh`, `.aws`, `.config`, `AppData`, browser profiles or package caches. Create a folder
> for work with agents, for example `C:\burrow-work`, put only the projects you want agents to
> see in it, and share only that.
>
> **The WSL backend is highly discouraged for now.** By default WSL mounts every Windows drive
> into the distro (the whole host directory tree is visible), runs Windows programs from inside
> (interop), and lets the default user become root. If you must use WSL, use a distro of its own
> set up as in [WSL: a dedicated distro](#wsl-a-dedicated-distro-temporary-protection), which
> shares only the work folder and turns interop off. That setup is a **temporary protection**, not
> a boundary (see its warning). Prefer the Hyper-V backend, which shares only the folders you
> name. WSL stays on the list to be tested and hardened (exception E1).

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

For the WSL test only: use a dedicated distro set up as in
[WSL: a dedicated distro](#wsl-a-dedicated-distro-temporary-protection) before putting any real
work in it.

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

## WSL: a dedicated distro (temporary protection)

> **Temporary protection, not a security boundary.** These settings live inside the distro, in
> files that root there can change, and every WSL 2 distro runs in the **same VM and kernel**. They
> stop accidents and casual access (an agent listing `/mnt/c`, running `cmd.exe`), not a
> determined attacker who becomes root in the distro or escapes into the VM: root can mount `C:`
> again, and a kernel escape reaches every distro. They also do nothing for the other distros you
> have. Use them until the hardened machine image exists (ROADMAP, E1), and prefer Hyper-V. Serves
> E1 and the organisation's controls CFG-02 (secure baseline configurations) and IAC-30 (least
> privilege); SCF is credited as in secure-standard-platforms' NOTICE.md.

What it gives you: a WSL distro used **only** for burrow, where Windows drives are not mounted
except one work folder, Windows programs cannot be started from Linux, and the daily user has no
`sudo`. Podman runs rootless inside it, as in "Status on Windows" above.

**1. A distro of its own** (PowerShell, as your normal user). Do not reuse the distro you use for
other work: its files, keys and history would be in reach.

```powershell
mkdir C:\burrow-work                       # the only Windows folder the distro will see
wsl --list --online                         # check the name: Ubuntu-24.04 (Podman 4.9+, netavark)
wsl --install Ubuntu-24.04 --no-launch      # if you already have an Ubuntu-24.04 distro, see below
wsl --export Ubuntu-24.04 $env:TEMP\ubuntu-24.04.tar
wsl --import burrow C:\wsl\burrow $env:TEMP\ubuntu-24.04.tar
wsl --unregister Ubuntu-24.04               # ONLY if you installed it just now: this deletes it
del $env:TEMP\ubuntu-24.04.tar
```

If you already use an `Ubuntu-24.04` distro, skip the install, export and unregister lines and
import a clean root filesystem instead (`wsl --import burrow C:\wsl\burrow <file.tar>`): an
export of your own distro would carry your files. *Whether a distro installed with `--no-launch`
can be exported before its first start is unverified.* Recent WSL versions also accept a name at
install (`wsl --install Ubuntu-24.04 --name burrow`); *that option is not on Microsoft's
basic-commands page (checked 2026-11-23), so it is unverified*.

**2. A user without sudo, and Podman** (inside the new distro; an imported distro starts as root):

```sh
wsl -d burrow                               # from PowerShell; you are root
apt update && apt install -y podman
useradd -m -s /bin/bash burrow              # the daily user: not in the sudo group
passwd -l root                              # no root password inside the distro
id -u burrow; id -g burrow                  # use these for uid= and gid= in step 4
```

**3. `/etc/wsl.conf`** (still as root in the distro):

```ini
[automount]
enabled = false
mountFsTab = true

[interop]
enabled = false
appendWindowsPath = false

[user]
default = burrow

[boot]
systemd = true
```

- `automount enabled = false`: no `C:`, `D:` ... under `/mnt`; `mountFsTab = true` still mounts
  the one folder in `/etc/fstab` (step 4).
- `interop enabled = false`: Linux cannot start Windows programs (`cmd.exe`, `powershell.exe`,
  `wsl.exe`), and `appendWindowsPath = false` keeps Windows folders out of `PATH`.
- `user default = burrow`: sessions start as the user without sudo.
- `boot systemd = true` is optional: it lets Podman use the systemd cgroup manager instead of the
  `cgroupfs` setting above.

**4. `/etc/fstab`: share only the work folder** (one line):

```
C:\134burrow-work /mnt/burrow-work drvfs rw,noatime,uid=1000,gid=1000,umask=077,metadata 0 0
```

`\134` is a backslash in fstab's escaping, so the source reads `C:\burrow-work`. Create the mount
point with `mkdir -p /mnt/burrow-work`. *The fstab line for a single folder is not in Microsoft's
wsl-config page (checked 2026-11-23); it follows the DrvFs options listed there and is
unverified until it is tested here (step 3 under "What to record").*

**5. Restart the distro and check** (PowerShell):

```powershell
wsl --terminate burrow                      # wait about 8 seconds before starting it again
wsl -d burrow -- sh -c 'id; ls /mnt; ls /mnt/burrow-work; mount | grep -i drvfs'
wsl -d burrow -- cmd.exe /c ver             # must fail: interop is off
wsl -d burrow -- sudo -n true               # must fail: no sudo
```

The expected answer: the `burrow` user (not root), `/mnt` holds only `burrow-work` (and `wsl`/`wslg`
entries WSL adds itself), one `drvfs` mount, and both commands fail.

**Things to know:**
- **Keep repositories on the distro's own disk** (`~/src`), not in `/mnt/burrow-work`: the share
  is DrvFs, has the same lock problem as the Podman machine shares ("Known issues"), and is slow
  with many small files. Use the work folder to move files in and out.
- **Root is still one command away from Windows**: `wsl -d burrow -u root` works for anyone logged
  in as you on Windows. That is accepted (the Windows user is trusted); with interop off, nothing
  inside the distro can run `wsl.exe` to do the same.
- **Network:** WSL's default NAT mode forwards ports bound in the distro to `localhost` on
  Windows (`localhostForwarding`, in `%UserProfile%\.wslconfig`, global to every distro). burrow
  publishes ports only on request (`burrow port`); do not turn on `networkingMode=mirrored` for
  this distro's sake, it changes every distro.
- **Other distros** share the VM and kernel and keep their own settings: harden or remove the ones
  you do not need.

Sources (checked 2026-11-23): Microsoft, [Advanced settings configuration in
WSL](https://learn.microsoft.com/en-us/windows/wsl/wsl-config) (updated 2026-10-16:
`[automount] enabled`, `mountFsTab`, DrvFs options, `[interop]`, `[user] default`, `[boot]
systemd`, the 8-second rule, `.wslconfig` networking), and [Basic commands for
WSL](https://learn.microsoft.com/en-us/windows/wsl/basic-commands) (updated 2025-12-01:
`--install`, `--no-launch`, `--export`, `--import`, `--unregister`; `[user] default` for imported
distros).

## WSL hardening (E1, to build)

The steps above, as a machine image of our own built and pinned in CI, with root locked and the
settings checked at start. Root inside could undo them, so the shared kernel and network stay a
recorded risk.
