# Librescoot Version Service

Part of the [Librescoot](https://librescoot.org/) open-source platform.

The Version Service is a one-shot Go program that publishes the local operating-system release metadata to a Redis or Valkey hash. Platform images install a board-specific systemd unit that writes either `version:mdb` or `version:dbc`.
## Capabilities

- Reads key/value fields from `/etc/os-release`, lowercases their keys, and writes them to a configured hash.
- Derives `serial_number` and `serial_number_real` when the required i.MX OTP values can be read.
- Reads OTP values first from `/sys/bus/nvmem/devices/imx-ocotp0/nvmem`, then falls back to `/sys/fsl_otp/HW_OCOTP_CFG0` and `/sys/fsl_otp/HW_OCOTP_CFG1`.
- Fails when `/etc/os-release` or the Redis/Valkey endpoint cannot be read; an unavailable OTP value is logged as a warning and does not prevent release metadata from being published.

## Operation and interfaces

The program runs once: it parses `/etc/os-release`, connects to Redis/Valkey, writes all fields with `HSET`, and exits. It does not subscribe to commands or remain resident.

The packaged MDB unit runs:

```sh
/usr/bin/version-service -hash="version:mdb"
```

It requires and starts after `valkey.service`. The DBC unit writes `version:dbc`, starts after the network target, and retries on failure. In both cases the recipe installs the selected unit as `librescoot-version.service`.

Consumers can read the published values directly, for example:

```sh
redis-cli HGET version:mdb version_id
redis-cli HGET version:dbc variant_id
```

`version_id` and `variant_id` are values supplied by the local `/etc/os-release`; the service does not define or transform their values.

## Configuration

| Flag | Default | Purpose |
| --- | --- | --- |
| `-redis` | `192.168.7.1:6379` | Redis/Valkey server address |
| `-hash` | `os-release` | Destination hash |
| `-version` | — | Print the build version and exit |

No configuration file or environment-variable interface is implemented.

## Build and test

A Go toolchain is required. The default target produces a Linux ARMv7 binary.

```sh
make build       # bin/version-service for ARMv7
make build-host  # bin/version-service for the current host
make test
```

`make lint`, `make fmt`, `make deps`, and `make clean` are also available.

## Deployment and runtime dependencies

The image recipe installs `/usr/bin/version-service` and one board-specific unit at `/usr/lib/systemd/system/librescoot-version.service`. The service needs readable OS-release data and network access to the configured Redis/Valkey endpoint. OTP-derived serial fields additionally require one of the supported NVMEM or OTP interfaces; their absence only omits those derived fields.

Use the packaged unit on a target:

```sh
systemctl status librescoot-version.service
journalctl -u librescoot-version.service
```

## Operational and security notes

- The destination hash is configurable. Use a dedicated, trusted Redis/Valkey endpoint and avoid pointing it at a shared untrusted instance.
- The service publishes operating-system metadata and, when available, device serial identifiers. Apply the same Redis/Valkey access controls used for other vehicle metadata.
- Because it is one-shot, rerun or restart `librescoot-version.service` after replacing `/etc/os-release` if consumers need the new values before the next boot.

## License

This project is licensed under the [GNU Affero General Public License v3.0](LICENSE).

Made with ❤️ by the Librescoot community
