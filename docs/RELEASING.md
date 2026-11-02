# Releasing

Releases are cut by hand from a clean `main`. CI never publishes.

1. Confirm `main` is green and the example is current:

   ```sh
   git switch main && git pull
   make all
   make check-example
   ```

2. Build the archives and checksums for the new version:

   ```sh
   make release VERSION=v0.1.0
   make verify-release
   ```

   This cross-compiles `darwin/arm64`, `darwin/amd64`, `linux/amd64`, and
   `linux/arm64` with `CGO_ENABLED=0`, `-trimpath`, and
   `-ldflags "-s -w -buildid= -X .../cli.Version=v0.1.0"`, packs each binary
   with `LICENSE` and `README.md` into
   `dist/profile-forge_<version>_<os>_<arch>.tar.gz`, and writes
   `dist/checksums.txt` with SHA-256 sums.

3. Smoke test the host binary from its archive:

   ```sh
   tar -xzf dist/profile-forge_v0.1.0_darwin_arm64.tar.gz -C dist
   dist/profile-forge_v0.1.0_darwin_arm64/profile-forge version
   ```

4. Publish:

   ```sh
   gh release create v0.1.0 dist/*.tar.gz dist/checksums.txt \
     --title "v0.1.0" --notes "..."
   ```

5. Verify the published assets match the local checksums:

   ```sh
   mkdir -p verify && cd verify
   gh release download v0.1.0 --repo adeoluo/profile-forge
   shasum -a 256 -c checksums.txt
   ```

## Homebrew

A formula needs, per platform, the archive URL and its SHA-256 from
`checksums.txt`:

```
https://github.com/adeoluo/profile-forge/releases/download/<version>/profile-forge_<version>_<os>_<arch>.tar.gz
```

Each archive contains one top-level directory,
`profile-forge_<version>_<os>_<arch>/`, holding `profile-forge`, `LICENSE`, and
`README.md`.
