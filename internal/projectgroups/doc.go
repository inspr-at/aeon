// SPDX-License-Identifier: AGPL-3.0-only

// Package projectgroups serves shared project groups (AEON-136): /api/project-groups.
//
// Groups on the Projects page are personal by default and live in the person's
// preferences (the web app's "project-groups" key). A workspace admin can share
// a group, which turns it into this entity: a name, a position for the default
// order, and its member projects. A project is in at most one shared group.
// Everyone in the tenant reads shared groups; only admins create, rename,
// reorder, delete them or change what is in them. Every write appends an event
// in the same tenant transaction, and UndoHandlers reverses each event type
// through POST /api/events/{id}/undo.
//
// Wiring (cmd/aeon serve): mount New(pool) with the other modules and register
// events.WithUndoHandlers(projectgroups.UndoHandlers()).
package projectgroups
