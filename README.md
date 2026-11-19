# aTerm

English | [简体中文](README.zh-CN.md)

aTerm is a terminal application built for server management, with a focus on SSH workflows. It brings local terminals, SSH connections, server organization, and remote file editing into one workspace.

## Features

- **Host management** — organize connections in directories, search hosts, and import SSH connection settings.
- **SSH terminals** — work with tabbed connections, split terminal layouts, managed SSH keys, and jump hosts.
- **SSH tunnels** — save and control local port forwarding for private services, reuse jump hosts, and keep tunnels running independently of terminal tabs.
- **Remote files** — browse files over SFTP, upload and download files, track transfers, and edit remote text files with Monaco Editor.
- **Local terminals** — open a local shell alongside remote connections.
- **Workspace extensions** — install and manage plugins with permission controls.
- **Personalization and backups** — manage terminal themes and preferences, and export or restore encrypted backups.

## Built for everyday server work

Move between local shells and remote servers, keep connections organized, and work with terminals and remote files in the same workspace. aTerm follows native macOS interface conventions to keep everyday operations familiar and straightforward.

## Terminals in one workspace

Open local shells and SSH sessions side by side. Use tabs to switch between connections and split terminal layouts to work across multiple sessions.

## Keep servers organized

Group connections into directories and find hosts with search. Import SSH connection settings, manage SSH keys, and connect through jump hosts.

## Access private services

Open **Tunnels** in the bottom view bar of a saved SSH connection, or use **SSH tunnels** in the sidebar for the global view. Save a local address and port, select an SSH host, and enter the target address as reached from that host. Start the tunnel and connect your browser or database tool to the local address. Tunnels stop when aTerm quits; saved configurations are included in encrypted backups. See [SSH tunnel management](docs/ssh-tunnels.md) for lifecycle and troubleshooting details.

## Work with remote files

Browse server files over SFTP without leaving your workspace. Upload and download files, follow transfer progress, and edit remote text files with the built-in editor.

## Make the workspace your own

Choose terminal themes and adjust preferences to suit your workflow. Extend the workspace with plugins, control their permissions, and export or restore encrypted backups.
