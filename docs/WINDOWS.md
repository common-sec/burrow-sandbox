# Windows

burrow runs on Windows as a normal program; `podman` is the remote client of a **Podman machine**
(a Linux VM) where the sandboxes run. Two backends, to be tested side by side (J, 2026-11-18):

| | WSL backend (first: exception E1) | Hyper-V backend |
|---|---|---|
| Windows editions | all, Home included | Pro, Enterprise, Education |
| Rights | none beyond WSL | *Hyper-V Administrators* (or admin) to create the machine |
| Isolation from the rest of Windows | one VM and kernel shared with every WSL distro; Windows drives mounted unless turned off; interop runs Windows programs unless turned off | a VM of its own |
| Folders | Windows paths are reachable through the WSL mounts | only folders shared at `podman machine init -v` (9p) |
| Ports | published ports reach `localhost` on Windows and from other distros | forwarded to `localhost` on Windows |

*From documentation, to verify on a real machine.*

## Try it

```powershell
# Once: a machine (pick one)
podman machine init --provider wsl burrow-wsl            # WSL backend
podman machine init --provider hyperv -v C:\Users\me\src:/mnt/src burrow-hv   # Hyper-V, admin
podman machine start burrow-wsl
podman system connection default burrow-wsl

# In a clone of this repository
mise install
mise run images
go build -o burrow.exe ./cmd/burrow
.\burrow.exe up examples\envs\dev.env       # set WORKSPACE="C:\Users\me\src\project" first
.\burrow.exe exec dev -- curl -sI https://pypi.org/simple/
.\burrow.exe --dry-run up examples\envs\dev.env   # shows exactly what is sent to podman
```

## What to record for each backend (into TESTING.md)

1. `mise run e2e` from Git Bash or WSL against the machine, or the commands above by hand.
2. Workspace: a `C:\` folder mounted at `/work`; file owner and permissions inside; speed of
   reading many small files.
3. Ports: `burrow port dev 8080` reachable from a Windows browser at `http://127.0.0.1:8080`.
4. Visibility: from another WSL distro, can you see the sandbox's processes, files or ports?
5. Rights needed, and what an enterprise policy blocks.

## WSL hardening (E1, to build)

A machine image of our own: `/etc/wsl.conf` with `[automount] enabled=false`, `mountFsTab=false`,
`[interop] enabled=false`, `appendWindowsPath=false`; an unprivileged default user with no sudo
and root locked. Root inside could undo it, so the shared kernel and network stay a recorded risk.
