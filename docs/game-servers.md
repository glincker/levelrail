---
title: "Minecraft on a small server"
icon: "/assets/icons/download-simple.svg"
sidebarTitle: "Game servers"
description: "Run a Minecraft Java server on a small VM with Levelrail: rough memory sizing, TCP and UDP ports, the two firewall layers, and backups."
"og:url": "https://docs.levelrail.com/game-servers"
---

This page covers running one Minecraft Java server from the Levelrail template on a small VM. It is guidance, not a game panel: Levelrail runs the container, keeps its data volume and can back it up. It does not manage players, mods or a console UI.

The template uses the community `itzg/minecraft-server` image. Its own documentation covers every server setting: [docker-minecraft-server](https://github.com/itzg/docker-minecraft-server). See the generated [Minecraft Server template page](/self-host/minecraft) for the template's current ports, volumes and variables.

## Sizing

The numbers below are rough guidance from how Java servers behave in general. They are not benchmarks, and Levelrail has not measured them. Memory use grows with players, view distance, loaded chunks and mods or plugins.

| Players online | Java heap to start from | Total VM RAM to plan for |
| --- | --- | --- |
| 1 to 5 | 2 GB | 4 GB |
| 5 to 10 | 3 to 4 GB | 6 GB |
| 10 to 20 | 4 to 6 GB | 8 GB |

The template sets a recommended memory figure. The Java heap is only part of the process: leave room for the JVM itself, the operating system and the Levelrail control plane. Lowering the view distance and simulation distance in the server settings reduces both memory and CPU. Raise the heap only if you see out-of-memory errors or long pauses.

## Ports

A Minecraft Java server listens on TCP 25565. Some other games use UDP. Check the game's documentation, then publish the right protocol for the game. A UDP service is published as `HOST:CONTAINER/udp`, see [UDP published ports](/udp-ports).

Direct game ports do not go through the HTTP reverse proxy. A published port follows the exposure rules in [App ports and exposure](/app-exposure): it is bound privately by default, and you choose to make it public.

## Two firewall layers

Players can connect only if both layers allow the port:

1. The cloud firewall: security group, VCN security list or network security group at your provider. Levelrail cannot change this one.
2. The host firewall on the VM: ufw, firewalld, nftables or iptables. See [Host firewall](/host-firewall).

Docker-published ports often skip the host `INPUT` chain, so the host layer may not block them, but do not assume. Test from a different network. For Oracle specifics see [Install on Oracle Cloud](/install-on-oracle-cloud).

## Backups

A world is a set of files that change while the server runs. Copying them mid-write can give a corrupted backup. The usual approach is to pause world saving, flush to disk, take the backup, then resume saving. Check the upstream docs above for how to run server console commands in that image.

Levelrail can back up an app volume to an S3-compatible target on a schedule, see [Backup targets and volume backups](/backups-and-storage). Levelrail does not run pre and post backup hooks for you today, so if you need a consistent world, stop the server before the backup runs or take the backup at a time when nobody is online. Rehearse a restore before you rely on it: see [Prove your backups work](/prove-your-backups-work).

## Arm VMs

On an Arm VM, check that the image tag you use has a `linux/arm64` build (for example with `docker buildx imagetools inspect <image:tag>`). Levelrail has not verified this for every game image, so check each one before installing. See [Template catalog](/template-catalog).
