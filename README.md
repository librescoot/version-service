# Librescoot Version Service

A simple Go service for Librescoot that reads OS release information from `/etc/os-release` and stores it in Redis.

Part of the [Librescoot](https://librescoot.org/) open-source platform.

## Features

- Reads system version information from `/etc/os-release`
- Stores the information in a Redis hash with lowercase keys
- Configurable Redis server address and hash name
- Runs as a one-shot systemd service after network is available

## Building

The project includes several build targets:

- `make build` - Build for the host architecture
- `make build-arm` - Build for ARMv7l (without optimization)
- `make dist` - Build an optimized and stripped binary for ARMv7l (stripped and optimized)
- `make clean` - Remove built binaries

## Installation

To install the service manually:

1. Build the optimized binary for ARMv7l:
   ```bash
   make dist
   ```

2. Copy the binary to the target system:
   ```bash
   scp version-service root@target-device:/usr/bin/
   ```

3. Copy the appropriate systemd service file:
   ```bash
   # For MDB (runs after Redis service)
   scp version-service-mdb.service root@target-device:/etc/systemd/system/
   
   # For DBC
   scp version-service-dbc.service root@target-device:/etc/systemd/system/
   ```

4. On the target system, reload systemd and enable the service:
   ```bash
   systemctl daemon-reload
   
   # For MDB
   systemctl enable version-service-mdb.service
   
   # For DBC
   systemctl enable version-service-dbc.service
   ```

## Usage

The service accepts the following command-line arguments:

- `-redis` - Redis server address (default: "192.168.7.1:6379")
- `-hash` - Redis hash name to store the values (default: "os-release")

Example:

```bash
version-service -redis="192.168.7.2:6379" -hash="system-info"
```

## Systemd Unit Files

Two systemd unit files are provided:

1. `version-service-mdb.service` - For the MDB (Main Dashboard Board)
   - Runs after Redis service is available
   - Stores data in the `version:mdb` hash

2. `version-service-dbc.service` - For the DBC (Dashboard Controller)
   - Runs after network is available
   - Stores data in the `version:dbc` hash

## License

This project is dual-licensed. The source code is available under the
[GNU Affero General Public License v3.0][agpl-3.0].
The maintainers reserve the right to grant separate licenses for commercial distribution; please contact the maintainers to discuss commercial licensing.

[![AGPL v3][agpl-image]][agpl-3.0]

[agpl-3.0]: https://www.gnu.org/licenses/agpl-3.0.en.html
[agpl-image]: https://www.gnu.org/graphics/agplv3-88x31.png
