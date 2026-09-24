// SPDX-License-Identifier: AGPL-3.0-only

// Package quotepdf renders the frozen QP3 document in an isolated Chromium
// session. The coordinator passes the built web filesystem (web.Static) to
// Render. Its quote-print.html entry mounts QuoteDocument read-only and signals
// readiness only after bundled fonts and whole-block pagination settle.
//
// Chromium belongs in the Aeon image. The QP5 local linux/amd64 image measured
// 1.11 GB with `docker image ls`; apk reported 709.5 MiB installed for the
// runtime's 196 packages. A render is budgeted at 300-500 MiB resident memory
// until release QA measures peak RSS on the target host. The coordinator must
// raise the host memory limit and bound concurrent renders before enabling
// confirmation workers. The browser receives no service credentials; the
// print page's CSP permits only loopback assets and inline document data.
package quotepdf
