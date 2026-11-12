// Package cloudconn holds named cloud account connections (preview). It stores
// non-secret settings and a credential reference only, and probes health through
// a read-only provider interface. No provider here creates or changes cloud resources.
package cloudconn
