# Golem

> [!IMPORTANT] 
> Plugin Live Reloading is for Development Only!
>
> Golem's live plugin reloading and auto-update features are intended strictly for development workflows. **Do not use this tool in production to perform automatic updates.** [PaperMC team discourages the use of automated update mechanisms][https://docs.papermc.io/misc/downloads-api/].

Golem is a powerful command-line tool designed to streamline Minecraft plugin development. Its primary focus is to provide seamless **live reloading of plugins** during development, automatically detecting changes and restarting your server to reflect updates instantly.

## Features

- 🛠️ **Live Reloading for Plugin Development:**
  - Instantly reload plugins on code changes with the `--watch` mode.
  - Automatically copies new plugin builds to the server's plugins directory and restarts the server.
- 🚀 Supports multiple server types:
  - Paper
  - Purpur
- 🔄 Automatic server updates (for development convenience only)


## Quick Start

1. Build or download Golem (no official releases yet; build from source if needed)
2. Place it in your plugin development workspace
3. Create a `golem-config.json` file (see below)
4. Start Golem in **live reload mode**:

```bash
golem --auto-start --watch ./build/libs
```

## Installation

Clone or download this repository and build the binary for your platform. Place the executable in your plugin development directory.

## Usage: Live Plugin Reloading (Development Mode)

The core feature of Golem is the **live plugin reloading mode**:

```bash
golem --auto-start --watch path/to/plugin/directory
```

- The `--watch` flag enables plugin development mode: Golem watches the specified directory for `.jar` changes, automatically copies updated plugins to your server's `plugins` folder, and restarts the server to apply changes.
- The `--auto-start` flag ensures the server starts automatically after updates.

> ⚠️ **Warning:** Do not use `--watch` or any auto-update features in production. This is for developer convenience only!

## Configuration

Create a `golem-config.json` file in your project root. Example:

```json
{
    "serverType": "paper",
    "serverVersion": "1.21.5",
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
| allowExperimentalBuilds | Allow experimental server builds (paper) | false |

## Command Line Options

| Flag | Description |
|------|-------------|
| --config | Path to config file |
| --watch | Path to plugin development directory |
| --auto-start | Automatically start server after update |

## Development Features

- **Live Reloading**: When using the `--watch` flag, Golem automatically detects changes in your plugin directory and restarts the server to apply the changes.
- **Build Integration**: Automatically copies new plugin builds to the server's plugins directory.
