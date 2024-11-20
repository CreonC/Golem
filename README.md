# Golem

> [!IMPORTANT]
> While most of Golem's features are tested (?) and work as expected, there may be some bugs and unexpected behavior. Use at your own risk.
> I'm too lazy to write unit tests for now.

A powerful command-line tool for managing and developing Minecraft (plugins) servers with ease. Golem automates the process of downloading, configuring, and maintaining various types of Minecraft servers, making server management and plugin development more efficient.

## Features

- 🚀 Supports multiple server types:
  - Paper
  - Vanilla --SOON
  - Purpur
- 🔄 Automatic server updates (and plugins in the future)
- 🛠️ Plugin development mode with live-reloading
- ⚙️ Flexible configuration system

## Quick Start

1. Download the latest Golem release (There's none, you'll have to build it yourself)
2. Create a configuration file (config.json)
3. Run Golem:
```bash
golem --auto-start
```

## Installation

Download the latest release for your platform from the releases page and place it in your desired location.

## Usage

### For Server Administrators

Basic server management:
```bash
golem --auto-start
```

Using a custom config file:
```bash
golem --config path/to/config.json --auto-start
```

### For Plugin Developers

Enable live-reloading of your plugin during development:
```bash
golem --auto-start --watch path/to/plugin/directory
```

## Configuration

Create a `golem-config.json` file with the following structure:

```json
{
    "serverType": "paper",
    "serverVersion": "1.21.3",
    "buildNumber": 44,
    "javaPath": "java",
    "minRam": "1G",
    "maxRam": "4G",
    "serverPath": "./server",
    "allowExperimentalBuilds": true
}
```

### Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| serverType | Type of server (paper/vanilla/purpur) | paper |
| serverVersion | Minecraft version | latest |
| buildNumber | Build number for the server (if applicable) | latest |
| javaPath | Path to Java executable | "java" |
| minRam | Minimum RAM allocation | "1G" |
| maxRam | Maximum RAM allocation | "4G" |
| serverPath | Directory for server files | "./server" |
| allowExperimentalBuilds | Allow experimental server builds | false |

## Command Line Options

| Flag | Description |
|------|-------------|
| --config | Path to config file |
| --watch | Path to plugin development directory |
| --auto-start | Automatically start server after update |

## Development Features

- **Live Reloading**: When using the `--watch` flag, Golem automatically detects changes in your plugin directory and restarts the server to apply the changes.
- **Build Integration**: Automatically copies new plugin builds to the server's plugins directory.
