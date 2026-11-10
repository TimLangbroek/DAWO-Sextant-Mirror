# API reference

<!-- Made from internal/http/api/openapi.json; do not edit by hand. Regenerate with
     go test ./internal/http/api -run TestTheAPIReferenceIsCurrent -update -->

Every console serves this contract as OpenAPI 3.0 at `GET /api/v1/openapi.json`, without
a credential. This page is the same document for reading. Contract version: 1.

Machine contract of the Sextant fleet control-plane.

AUTHENTICATION. A Bearer token (personal or service), or a browser session with X-CSRF-Token on mutations. Both resolve through the same wrapper; no endpoint authenticates its own way.

ERRORS. Every failure answers application/json with the Error schema - including the refusals raised before a handler runs and a path matching no route. A 500 always carries the literal string "internal error"; the detail is in the server log, deliberately, so an error response can never leak internals.

EMPTY LISTS. A list endpoint answers [] and never null, whether or not the fleet is empty.

VERSIONING, from the 1.0 freeze:

* ADDITIVE ONLY within /api/v1. New endpoints, new optional request fields and new response fields may appear at any time. A client MUST ignore response fields it does not know - that is the whole basis of the promise.
* NEVER within /api/v1: removing or renaming an endpoint, a field or an enum value; narrowing a type; making an optional request field required; or changing the status code an operation answers for the same condition.
* If something genuinely must break, it happens under /api/v2 and the two run side by side. /api/v1 then enters a DEPRECATION WINDOW of at least twelve months from the day v2 ships, announced in the release notes and carried on every deprecated operation as `deprecated: true` in this document.
* A security fix may break the rules above. That is the one exception, it is announced as such, and the reason is published.

This spec is verified against the running server: CI fails if a route exists without a spec entry, if a spec entry has no route, if an operation does not document its errors, or if a documented response shape is not what the server writes.

## Operations

| Operation | What it does |
|---|---|
| `GET /api/v1/access` | List role bindings |
| `POST /api/v1/access` | Grant a role binding |
| `DELETE /api/v1/access` | Revoke a role binding |
| `PUT /api/v1/apps` | Replace an app list at a scope |
| `POST /api/v1/assignments` | Assign a policy to a scope |
| `DELETE /api/v1/assignments` | Unassign a policy |
| `PUT /api/v1/assurance` | Configure audit controls |
| `GET /api/v1/audit` | Configuration audit trail |
| `GET /api/v1/changes` | List change requests |
| `POST /api/v1/changes` | Open a change request |
| `GET /api/v1/changes/{id}` | One change request |
| `POST /api/v1/changes/{id}/abandon` | Abandon a change |
| `GET /api/v1/changes/{id}/diff` | Unified diff the change would apply |
| `POST /api/v1/changes/{id}/edits` | Apply a gated edit on the change branch |
| `POST /api/v1/changes/{id}/merge` | Merge a ready change (owner) |
| `POST /api/v1/changes/{id}/submit` | Run the build gate |
| `GET /api/v1/devices` | List devices |
| `POST /api/v1/devices` | Enroll a device |
| `GET /api/v1/devices/{tag}` | Device with resolved configuration |
| `PATCH /api/v1/devices/{tag}` | Update device fields |
| `DELETE /api/v1/devices/{tag}` | Remove a device |
| `POST /api/v1/devices/{tag}/credential` | Re-issue a device credential |
| `POST /api/v1/devices/{tag}/intent` | Arm a remote action |
| `DELETE /api/v1/devices/{tag}/intent` | Clear a remote action |
| `POST /api/v1/devices/{tag}/reactivate` | Reactivate a retired device |
| `POST /api/v1/devices/{tag}/retire` | Retire a device |
| `GET /api/v1/directory/groups` | Browse IdP directory groups |
| `GET /api/v1/evidence` | Audit evidence export |
| `GET /api/v1/facts/{tag}` | Hardware facts of one device |
| `PUT /api/v1/filters/{id}` | Create or replace a filter |
| `DELETE /api/v1/filters/{id}` | Delete a filter |
| `GET /api/v1/fleet` | Full fleet document |
| `POST /api/v1/groups` | Create a group |
| `PATCH /api/v1/groups/{name}` | Update a group |
| `DELETE /api/v1/groups/{name}` | Remove a group |
| `GET /api/v1/hostkeys` | SSH host public keys of active devices |
| `GET /api/v1/me` | Who am I |
| `GET /api/v1/me/preferences` | My preferences |
| `PUT /api/v1/me/preferences` | Update my preferences |
| `PUT /api/v1/policies/{id}` | Create or replace a policy |
| `DELETE /api/v1/policies/{id}` | Delete a policy |
| `GET /api/v1/rollout` | Rollout state and ring convergence |
| `POST /api/v1/rollout` | Start a rollout (owner) |
| `DELETE /api/v1/rollout` | Cancel the active rollout |
| `PUT /api/v1/rollout/plan` | Replace the rollout ring plan |
| `POST /api/v1/rollout/tick` | Advance the rollout engine one step |
| `GET /api/v1/secret-refs` | List registered secret references |
| `POST /api/v1/secret-refs` | Register a secret reference (a name, never a value) |
| `DELETE /api/v1/secret-refs/{name}` | Unregister a secret reference |
| `POST /api/v1/settings` | Set a setting at a scope |
| `DELETE /api/v1/settings` | Clear a setting at a scope |
| `GET /api/v1/status` | Observed status of every device |
| `GET /api/v1/status/{tag}` | Observed status of one device |
| `GET /api/v1/tokens` | List your API tokens |
| `POST /api/v1/tokens` | Mint an API token (secret shown once) |
| `DELETE /api/v1/tokens/{id}` | Revoke an API token |

### `GET /api/v1/access`

List role bindings.

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `limit` | query | integer | How many items to return. Absent means all of them, which is the default and what a client written before paging existed will see. At most 1000 per request. |
| `offset` | query | integer | How many items to skip. An offset past the end is an empty array, not an error: a list that shrank between calls is ordinary. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | array of [AccessBinding](#accessbinding) | Role bindings the caller may see. Always an array. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/access`

Grant a role binding.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `DELETE /api/v1/access`

Revoke a role binding.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `PUT /api/v1/apps`

Replace an app list at a scope.

Body: {scope, kind: packages|flatpaks|overlays, names}. Names pass the injection firewall; the gate validates the result. Editor at the scope.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `POST /api/v1/assignments`

Assign a policy to a scope.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `DELETE /api/v1/assignments`

Unassign a policy.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `PUT /api/v1/assurance`

Configure audit controls.

Body: {requireFourEyes}. Org owner.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `GET /api/v1/audit`

Configuration audit trail.

Newest commits on the config repo (?limit=, default 100). Org-wide viewer: commits span every scope.

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `limit` | query | integer | How many items to return. Absent means all of them, which is the default and what a client written before paging existed will see. At most 1000 per request. |
| `offset` | query | integer | How many items to skip. An offset past the end is an empty array, not an error: a list that shrank between calls is ordinary. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | array of [AuditEntry](#auditentry) | Configuration audit trail, newest first. Bound with ?limit. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `GET /api/v1/changes`

List change requests.

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/changes`

Open a change request.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `GET /api/v1/changes/{id}`

One change request.

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/changes/{id}/abandon`

Abandon a change.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `GET /api/v1/changes/{id}/diff`

Unified diff the change would apply.

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/changes/{id}/edits`

Apply a gated edit on the change branch.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `POST /api/v1/changes/{id}/merge`

Merge a ready change (owner).

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `POST /api/v1/changes/{id}/submit`

Run the build gate.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `GET /api/v1/devices`

List devices.

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `limit` | query | integer | How many items to return. Absent means all of them, which is the default and what a client written before paging existed will see. At most 1000 per request. |
| `offset` | query | integer | How many items to skip. An offset past the end is an empty array, not an error: a list that shrank between calls is ordinary. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | array of [Device](#device) | Every device the caller may see. Always an array. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/devices`

Enroll a device.

| Answer | Body | Meaning |
|---|---|---|
| `200` | object with `credentialError`, `credential`, `notice`, `status`, `tag` | The device was enrolled. |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `GET /api/v1/devices/{tag}`

Device with resolved configuration.

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `PATCH /api/v1/devices/{tag}`

Update device fields.

Patch hardware, class, assignedUser, groups, labels or itam. Editor at the device scope; new group memberships also need editor there.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `DELETE /api/v1/devices/{tag}`

Remove a device.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `POST /api/v1/devices/{tag}/credential`

Re-issue a device credential.

Replaces the device credential (lost secret, re-image); the new secret is shown once. Editor at the device scope; refused for retired devices.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `POST /api/v1/devices/{tag}/intent`

Arm a remote action.

Body: {intent: lock|wipe, force?}. Wipe requires the device to be locked first unless force. Org owner; the intent is an audited commit the device pulls on check-in.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `DELETE /api/v1/devices/{tag}/intent`

Clear a remote action.

Cancel a pending intent (a found laptop). Org owner.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `POST /api/v1/devices/{tag}/reactivate`

Reactivate a retired device.

Returns the device to service and issues a fresh credential (shown once). Editor at the device scope.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `POST /api/v1/devices/{tag}/retire`

Retire a device.

Parks the device: the audit record stays, image builds, check-ins and rollout counting stop, and its credential is revoked. Editor at the device scope.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `GET /api/v1/directory/groups`

Browse IdP directory groups.

Groups matching ?q= (substring) from the configured LDAP directory, for access-binding pickers. Requires owner at some scope; read-only.

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `limit` | query | integer | How many items to return. Absent means all of them, which is the default and what a client written before paging existed will see. At most 1000 per request. |
| `offset` | query | integer | How many items to skip. An offset past the end is an empty array, not an error: a list that shrank between calls is ordinary. |

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `GET /api/v1/evidence`

Audit evidence export.

Self-contained JSON bundle for a period (?from=&to=, RFC 3339; default last 30 days): configuration commits, change requests with authorship, rollout promotions, and the controls in force. Org-wide viewer.

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `GET /api/v1/facts/{tag}`

Hardware facts of one device.

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `PUT /api/v1/filters/{id}`

Create or replace a filter.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `DELETE /api/v1/filters/{id}`

Delete a filter.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `GET /api/v1/fleet`

Full fleet document.

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/groups`

Create a group.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `PATCH /api/v1/groups/{name}`

Update a group.

Re-parent the group (cycle-safe) and/or change its IdP mapping. Org owner.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `DELETE /api/v1/groups/{name}`

Remove a group.

Only an empty leaf group with no references (devices, subgroups, assignments, bindings, rings) can be removed. Org owner.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `GET /api/v1/hostkeys`

SSH host public keys of active devices.

Returns [{tag, hostKey}] for every active, visible device with a recorded host key - the age recipients an overlay rekey run encrypts secrets for. Public key material only.

| Answer | Body | Meaning |
|---|---|---|
| `200` | array of [HostKey](#hostkey) | Recorded device host public keys. Always an array. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `GET /api/v1/me`

Who am I.

Identity claims plus the effective role per visible scope, derived live from the current document. Feeds the profile menu.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Me](#me) | The caller and their resolved roles. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `GET /api/v1/me/preferences`

My preferences.

Personal presentation settings (timezone, locale). Empty fields inherit the organisation default.

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `PUT /api/v1/me/preferences`

Update my preferences.

Body: {timezone?, locale?}. Timezone must be an IANA name; locale one of the shipped UI languages.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `PUT /api/v1/policies/{id}`

Create or replace a policy.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `DELETE /api/v1/policies/{id}`

Delete a policy.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `GET /api/v1/rollout`

Rollout state and ring convergence.

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/rollout`

Start a rollout (owner).

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `DELETE /api/v1/rollout`

Cancel the active rollout.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `PUT /api/v1/rollout/plan`

Replace the rollout ring plan.

Body: {plan} with rings [{group, soakMinutes, minHealthyPercent}]; null plan clears it. Org owner.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `POST /api/v1/rollout/tick`

Advance the rollout engine one step.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `GET /api/v1/secret-refs`

List registered secret references.

| Answer | Body | Meaning |
|---|---|---|
| `200` | array of [SecretRef](#secretref) | Registered secret references (names, never values). Always an array. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/secret-refs`

Register a secret reference (a name, never a value).

| Answer | Body | Meaning |
|---|---|---|
| `201` |  | Created |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `DELETE /api/v1/secret-refs/{name}`

Unregister a secret reference.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `POST /api/v1/settings`

Set a setting at a scope.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `DELETE /api/v1/settings`

Clear a setting at a scope.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `GET /api/v1/status`

Observed status of every device.

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `GET /api/v1/status/{tag}`

Observed status of one device.

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `GET /api/v1/tokens`

List your API tokens.

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/tokens`

Mint an API token (secret shown once).

| Answer | Body | Meaning |
|---|---|---|
| `201` |  | Created |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

### `DELETE /api/v1/tokens/{id}`

Revoke an API token.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Status](#status) | OK |
| `400` | [Error](#error) | The request could not be used: malformed body, unknown key, wrong-typed value, or a reference that resolves to nothing. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `409` | [Error](#error) | The write conflicts with the current state: a lost write race, or a change-request-required policy refusing a direct write. |
| `422` | [Error](#error) | The resulting configuration was refused by the nix evaluation gate. The message carries the evaluator's reason. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |
| `503` | [Error](#error) | A dependency the endpoint needs is not available (database, gate-runner, directory). Retry. |

## Schemas

### AccessBinding

| Field | Type | Always there | Meaning |
|---|---|---|---|
| `group` | string | yes | The directory group the binding grants to. |
| `role` | one of `viewer`, `editor`, `owner` | yes |  |
| `scope` | string | yes | Where the role holds: "org", or a group's scope such as "group:pilot". |

### AuditEntry

One entry of the configuration audit trail. It is git history, so it is append-only and complete.

| Field | Type | Always there | Meaning |
|---|---|---|---|
| `author` | string |  |  |
| `email` | string |  |  |
| `hash` | string |  | Commit hash. |
| `subject` | string |  | Commit subject: what changed, in the words the console wrote. |
| `when` | string |  |  |

### Device

A device as the fleet document holds it. This is the CONFIGURATION view; what a device reports about itself is /api/v1/status.

| Field | Type | Always there | Meaning |
|---|---|---|---|
| `assignedUser` | string |  | The person this device belongs to. Personal data - see docs/compliance/processing-register.md. |
| `class` | string |  | One of laptop, desktop, server, station. May be absent on a device a station discovered but nobody has classified. |
| `groups` | array of string |  | Group memberships, most general first. Always an array, never null. |
| `hardware` | string |  | Hardware profile name from the overlay. |
| `tag` | string | yes | Stable identifier. A lowercase slug; also the machine hostname. |

### Error

| Field | Type | Always there | Meaning |
|---|---|---|---|
| `error` | string | yes | Human-readable reason. Safe to show an operator: it never carries internals - a 500 is always the literal string "internal error", with the detail in the server log. |

### HostKey

| Field | Type | Always there | Meaning |
|---|---|---|---|
| `hostKey` | string | yes | The device's SSH host public key, as recorded at enrolment. |
| `tag` | string | yes | The device's tag. |

### Me

Who the caller is and what they may do. The roles map is resolved per request, never stored.

| Field | Type | Always there | Meaning |
|---|---|---|---|
| `ceiling` | string |  | Present only for a scoped token: the highest role it may reach regardless of the underlying grant. |
| `email` | string |  |  |
| `groups` | array of string |  | Directory groups. Always an array, never null. |
| `name` | string |  |  |
| `roles` | object |  | Resolved role per scope, e.g. {"org":"editor","group:pilot":"owner"}. Only scopes where the caller holds at least viewer appear. |
| `service` | boolean |  | True for a service account rather than a person. |
| `subject` | string |  | Identifier from the identity provider. |

### SecretRef

| Field | Type | Always there | Meaning |
|---|---|---|---|
| `description` | string |  |  |
| `name` | string | yes | The reference a setting uses. Never a value. |

### Status

The answer to a write that has nothing to return but its own success.

| Field | Type | Always there | Meaning |
|---|---|---|---|
| `status` | string | yes | What happened, e.g. "enrolled", "removed", "applied". |
