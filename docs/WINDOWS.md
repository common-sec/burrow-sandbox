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
make tools images test-images         # from Git Bash; or: mise install, podman build ...
mise exec -- go build -o burrow.exe ./cmd/burrow
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

## WSL hardening (E1, to build)

A machine image of our own: `/etc/wsl.conf` with `[automount] enabled=false`, `mountFsTab=false`,
`[interop] enabled=false`, `appendWindowsPath=false`; an unprivileged default user with no sudo
and root locked. Root inside could undo it, so the shared kernel and network stay a recorded risk.
