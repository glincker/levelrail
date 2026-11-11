// Package secretsource defines the single secret reference model
// (secret://conn/path#key), a Provider interface and a Resolver that routes
// by connection name. It imports only the standard library so existing
// providers can adapt to it without import cycles.
package secretsource
