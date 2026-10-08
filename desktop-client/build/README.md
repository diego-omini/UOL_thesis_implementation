# Build assets

This directory contains the application icon and platform metadata consumed by Wails. From `desktop-client`, build the release executable with:

```powershell
wails build
```

The executable is written to `build/bin/paper-portfolio-counselor.exe` on Windows. Product name, version, copyright, and executable name are configured in `wails.json`; platform templates under `darwin` and `windows` provide the remaining package metadata.
