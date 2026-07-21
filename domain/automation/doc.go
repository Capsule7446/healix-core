// Package automation owns durable automation assets and immutable published
// versions. Stable assets use opaque revisions for optimistic concurrency;
// version numbers and audit timestamps are never concurrency tokens.
package automation
