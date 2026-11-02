// Package gitx implements snapshot, diff, and restore by shelling out
// to the git binary. It is the ONLY place that calls git.
//
// Implemented in M2. M1 only reserves the package.
package gitx
