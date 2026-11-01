---
title: "Minecraft on a small server"
icon: "/assets/icons/download-simple.svg"
sidebarTitle: "Game servers"
description: "Run a Minecraft Java server on a small VM with Levelrail: rough memory sizing, TCP and UDP ports, the two firewall layers, and backups."
"og:url": "https://docs.levelrail.com/game-servers"
---

This page covers running one Minecraft Java server from the Levelrail template on a small VM. It is guidance, not a game panel: Levelrail runs the container, keeps its data volume and can back it up. It does not manage players, mods or a console UI.

The template uses the community `itzg/minecraft-server` image. Its own documentation covers every server setting: [docker-minecraft-server](https://github.com/itzg/docker-minecraft-server). See the generated [Minecraft Server template page](/self-host/minecraft) for the template's current ports, volumes and variables.

The template pins the image tag `2026.10.1` and offers two Java runtimes in the Java runtime field: `java21` (the default) and `java25`. You must enter `TRUE` for the EULA yourself, it is never accepted for you and the deploy form refuses an empty value. The server type is one of VANILLA, PAPER, FABRIC or FORGE.

The Minecraft version defaults to `1.21.11`, which boots on the Java 21 image (tested). `LATEST` stays selectable, but it can resolve to a release that needs a newer Java: the 26.x releases need Java 25, so on the Java 21 image the server exits with `UnsupportedClassVersionError`. Set the Java runtime to `java25` when you pick `LATEST` or a 26.x version. Both tags publish amd64 and arm64.

RCON is on with a generated password (kept as a secret) and its port is not published, so run `levelrail-cli apps console <app>` for an interactive RCON prompt, or `levelrail-cli apps console <app> -- list` for one command. The password stays in the container's environment and is never printed or put on a command line. Other game templates declare no console yet, use `apps exec` for them.

## Sizing

The numbers below are rough guidance from how Java servers behave in general. They are not benchmarks, and Levelrail has not measured them. Memory use grows with players, view distance, loaded chunks and mods or plugins.

| Players online | Java heap to start from | Total VM RAM to plan for |
| --- | --- | --- |
| 1 to 5 | 2 GB | 4 GB |
| 5 to 10 | 3 to 4 GB | 6 GB |
| 10 to 20 | 4 to 6 GB | 8 GB |

The template's JVM heap defaults to 2G and the container memory limit to 2560M, which is 25 percent above the heap. If you raise the heap, raise the container limit field with it to keep that headroom; a limit left at the template default is not scaled for you. The template also sets a recommended memory figure. The Java heap is only part of the process: leave room for the JVM itself, the operating system and the Levelrail control plane. Lowering the view distance and simulation distance in the server settings reduces both memory and CPU. Raise the heap only if you see out-of-memory errors or long pauses.

## Ports

A Minecraft Java server listens on TCP 25565. Some other games use UDP. Check the game's documentation, then publish the right protocol for the game. A UDP service is published as `HOST:CONTAINER/udp`, see [UDP published ports](/udp-ports).

Direct game ports do not go through the HTTP reverse proxy, and the Minecraft and Terraria templates declare their TCP port as a published port, so no web address is generated for them. A published port follows the exposure rules in [App ports and exposure](/app-exposure): it is bound privately (127.0.0.1) by default, and you choose to make it public in the app's Network settings. The templates do not make a game port public for you, because a template cannot declare a public bind and private by default is the platform's stance. After making it public, open the port in both firewall layers below.

## Two firewall layers

Players can connect only if both layers allow the port:

1. The cloud firewall: security group, VCN security list or network security group at your provider. Levelrail cannot change this one.
2. The host firewall on the VM: ufw, firewalld, nftables or iptables. See [Host firewall](/host-firewall).

Docker-published ports often skip the host `INPUT` chain, so the host layer may not block them, but do not assume. Test from a different network. For Oracle specifics see [Install on Oracle Cloud](/install-on-oracle-cloud).

## Backups

A world is a set of files that change while the server runs. Copying them mid-write can give a corrupted backup. The usual approach is to pause world saving, flush to disk, take the backup, then resume saving. Check the upstream docs above for how to run server console commands in that image.

Levelrail can back up an app volume to an S3-compatible target on a schedule, see [Backup targets and volume backups](/backups-and-storage). Each volume can have a pre-backup and a post-backup command, set with `levelrail-cli backups volumes policy set <app> <volume> --pre-hook '...' --post-hook '...'`. The post command always runs, even when the pre command fails or times out. A failed or timed-out pre command still produces a backup, recorded with an `inconsistent:` warning, so check that warning before you rely on the copy. Hooks are capped by `APP_BACKUP_HOOK_TIMEOUT_SECONDS` and `APP_BACKUP_HOOK_MAX_SECONDS`, and their output by `APP_BACKUP_HOOK_MAX_OUTPUT_BYTES`. The `--pause` option freezes the container while the archive streams instead.

For a Minecraft preset that runs `rcon-cli` (save-off, save-all flush, save-on), see the Minecraft example in [Prove your backups work](/prove-your-backups-work). Without hooks, stop the server before the backup runs or take the backup when nobody is online. Rehearse a restore before you rely on it: see [Prove your backups work](/prove-your-backups-work).

## Other game templates

Levelrail also ships templates for Minecraft Bedrock, Palworld, Terraria and Factorio, listed in the [Template catalog](/template-catalog). Bedrock listens on UDP 19132. Bedrock and Palworld are x86 game software, so on an arm64 host they run under emulation and need more CPU. Palworld needs about 16 GB of RAM for a few players, which does not fit a 12 GB host shared with other apps.

## Arm VMs

On an Arm VM, check that the image tag you use has a `linux/arm64` build (for example with `docker buildx imagetools inspect <image:tag>`). Levelrail has not verified this for every game image, so check each one before installing. See [Template catalog](/template-catalog).
