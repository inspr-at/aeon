// SPDX-License-Identifier: AGPL-3.0-only

// Package quotepdf renders the frozen QP3 document in an isolated Chromium
// session. The coordinator passes the built web filesystem (web.Static) to
// Render. Its quote-print.html entry mounts QuoteDocument read-only and signals
// readiness only after bundled fonts and whole-block pagination settle.
//
// Chromium belongs in the Aeon image. The QP5 local linux/amd64 image measured
// 1.11 GB with `docker image ls`; apk reported 709.5 MiB installed for the
// runtime's 196 packages. QW1 built the native linux/arm64 image locally,
// started aeon serve as UID 100 with AEON_ENV=dev against local Postgres, and
// rendered three A4 PDFs with TestBundledDocumentRendersA4 using the built web
// assets inside that running container. At 20 ms sampling, peak summed process
// VmRSS was 1206712 KiB (1178 MiB) across aeon, the test and Chromium; this
// counts shared pages in each process. The container's cgroup memory.peak was
// 291454976 bytes (278 MiB). Tini reaped the Chromium helpers after each run.
// AEON_PDF_CONCURRENCY defaults to 1 and accepts 1..4. Target-host release QA
// must measure its own peak before raising the render limit. The browser receives
// no service credentials; the
// print page's CSP permits only loopback assets and inline document data.
package quotepdf
