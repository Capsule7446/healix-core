// Package engine compiles one entry from an immutable execution RunSnapshot,
// resolves its typed parameter bindings (including frozen env. properties), and
// runs the resulting node Program with runtime ports supplied by Execution.
// Scheduling owns Run creation and latest-version freezing; Execution owns
// browser lifetime, worker fencing, and Evidence persistence.
package engine
