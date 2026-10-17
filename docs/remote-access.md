---
title: Local UI Access
description: Loopback UI access in the minimization fork
---

This fork serves its UI only on loopback (`127.0.0.1`, `::1`, or `localhost`).
Managed Caddy, public URLs, and public origins have been removed. Non-loopback
hosts and legacy public-hosting configuration are rejected, including when
authentication is enabled.

Use `agentsview serve --port 47831 --no-browser` to choose a local port.
The inherited default is still 8080. The fixed high-port default is pending.
A high port does not prevent a normal local browser from accessing the UI.

SSH/HTTP synchronization and remote client connections remain under review.
This listener change does not remove those transports. Local API authentication
and origin protections remain subject to the fork requirements and audit.
