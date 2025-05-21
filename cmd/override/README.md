# override plugin

This plugin overrides container configurations according to ocispec config file.

oci Spec please refer to https://github.com/opencontainers/runtime-spec/blob/master/config.md

## Features
- Set container rlimit configurations
- Add container hooks
- Modify other container runtime parameters

## Configuration Example

Create `override.conf` file with following content (OCI spec format):

```json
{
  "ociVersion": "",
  "process": {
    "rlimits": [
      {
        "type": "NOFILE",
        "soft": 65536,
        "hard": 65536
      }
    ]
  }
}
```

## Usage
1. Prepare ocispec config file
2. Configure containerd to use this plugin
3. Configurations will be automatically applied when creating containers
