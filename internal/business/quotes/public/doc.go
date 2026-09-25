// SPDX-License-Identifier: AGPL-3.0-only

// Package public serves quote capability links. Its rate limiter keeps state
// in each process, so a multi-replica deployment must enforce a shared limit
// at ingress or in the application before exposing the public routes.
package public
