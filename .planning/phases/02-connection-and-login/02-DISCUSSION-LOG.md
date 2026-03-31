# Phase 2: Connection and Login - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-31
**Phase:** 02-connection-and-login
**Areas discussed:** Login API shape, Negotiation engine, Credential handling, Login error reporting

---

## Login API Shape

### Q1: How should callers configure and trigger login?

| Option | Description | Selected |
|--------|-------------|----------|
| Config struct + Login() | Caller fills a LoginConfig struct and calls conn.Login(ctx, cfg) | |
| Functional options | conn.Login(ctx, WithTarget(...), WithCHAP(...), ...) | ✓ |
| Builder pattern | NewLogin().Target(...).CHAP(...).Do(ctx, conn) | |

**User's choice:** Functional options
**Notes:** None

### Q2: Should Dial and Login be separate steps?

| Option | Description | Selected |
|--------|-------------|----------|
| Separate Dial + Login | Keep transport.Dial() and conn.Login() as distinct steps | ✓ |
| Both: separate + convenience | Separate steps AND a top-level Connect() | |
| Combined Connect() only | Single entry point hides transport details | |

**User's choice:** Separate Dial + Login
**Notes:** None

---

## Negotiation Engine

### Q3: How should RFC 7143 Section 13 key negotiation be implemented?

| Option | Description | Selected |
|--------|-------------|----------|
| Declarative key registry | Each key is a struct describing type, default, range. Generic engine processes all keys. | ✓ |
| Per-key handlers | Each key gets its own resolve function | |
| Hybrid: registry + overrides | Declarative for standard, per-key overrides for special semantics | |

**User's choice:** Declarative key registry
**Notes:** None

### Q4: Where should negotiated parameters live after login?

| Option | Description | Selected |
|--------|-------------|----------|
| NegotiatedParams struct | Typed struct with all resolved values, direct field access | ✓ |
| map[string]string | Raw key-value map mirroring wire format | |
| Both: struct + raw map | Typed struct for common keys, raw map for unusual/vendor keys | |

**User's choice:** NegotiatedParams struct
**Notes:** None

---

## Credential Handling

### Q5: How should CHAP credentials be provided?

| Option | Description | Selected |
|--------|-------------|----------|
| Functional option | WithCHAP(user, secret) and WithMutualCHAP(user, secret, targetSecret) | ✓ |
| Callback interface | Authenticator interface with Challenge()/Response() methods | |
| Credential provider | CredentialProvider interface with GetCredentials(targetName) | |

**User's choice:** Functional option
**Notes:** None

---

## Login Error Reporting

### Q6: How should login failures be reported?

| Option | Description | Selected |
|--------|-------------|----------|
| Typed error with status | LoginError with StatusClass/StatusDetail mapping to RFC 7143 Section 11.13 | ✓ |
| Sentinel errors | ErrAuthFailed, ErrTargetNotFound, etc. | |
| Wrapped errors only | Standard fmt.Errorf wrapping | |

**User's choice:** Typed error with status
**Notes:** None

---

## Claude's Discretion

- Login state machine internal design
- CHAP crypto implementation details
- Text key-value encoding/decoding internals
- Internal package organization for login code
- NegotiatedParams embedding strategy

## Deferred Ideas

None
