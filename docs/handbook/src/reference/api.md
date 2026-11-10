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

Request body: [AccessBinding](#accessbinding).

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

Request body: [AccessRevoke](#accessrevoke).

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

Request body: [AppList](#applist).

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

Request body: [Assignment](#assignment).

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

Request body: [AssignmentRemoval](#assignmentremoval).

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

Request body: [Assurance](#assurance).

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
| `200` | array of [ChangeRequest](#changerequest) | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/changes`

Open a change request.

Request body: [NewChange](#newchange).

| Answer | Body | Meaning |
|---|---|---|
| `201` | [ChangeRequest](#changerequest) | Created. |
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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `id` | path | string | The change request's id, given when it was opened with POST /api/v1/changes. A lowercase slug. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | [ChangeRequest](#changerequest) | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/changes/{id}/abandon`

Abandon a change.

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `id` | path | string | The change request's id, given when it was opened with POST /api/v1/changes. A lowercase slug. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | [ChangeRequest](#changerequest) | OK |
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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `id` | path | string | The change request's id, given when it was opened with POST /api/v1/changes. A lowercase slug. |

| Answer | Body | Meaning |
|---|---|---|
| `200` |  | The change as a unified diff of fleet.json. |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/changes/{id}/edits`

Apply a gated edit on the change branch.

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `id` | path | string | The change request's id, given when it was opened with POST /api/v1/changes. A lowercase slug. |

Request body: [ChangeEdit](#changeedit).

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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `id` | path | string | The change request's id, given when it was opened with POST /api/v1/changes. A lowercase slug. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | [ChangeRequest](#changerequest) | OK |
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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `id` | path | string | The change request's id, given when it was opened with POST /api/v1/changes. A lowercase slug. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | [ChangeRequest](#changerequest) | OK |
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

Request body: [NewDevice](#newdevice).

| Answer | Body | Meaning |
|---|---|---|
| `201` | [DeviceWithCredential](#devicewithcredential) | The device was enrolled. |
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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `tag` | path | string | The device's tag: the name it has in the fleet, as GET /api/v1/devices lists it. A lowercase slug. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | [DeviceDetail](#devicedetail) | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `PATCH /api/v1/devices/{tag}`

Update device fields.

Patch hardware, class, assignedUser, groups, labels or itam. Editor at the device scope; new group memberships also need editor there.

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `tag` | path | string | The device's tag: the name it has in the fleet, as GET /api/v1/devices lists it. A lowercase slug. |

Request body: [DevicePatch](#devicepatch).

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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `tag` | path | string | The device's tag: the name it has in the fleet, as GET /api/v1/devices lists it. A lowercase slug. |

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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `tag` | path | string | The device's tag: the name it has in the fleet, as GET /api/v1/devices lists it. A lowercase slug. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | [IssuedCredential](#issuedcredential) | OK |
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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `tag` | path | string | The device's tag: the name it has in the fleet, as GET /api/v1/devices lists it. A lowercase slug. |

Request body: [DeviceIntent](#deviceintent).

| Answer | Body | Meaning |
|---|---|---|
| `200` | [IntentArmed](#intentarmed) | OK |
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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `tag` | path | string | The device's tag: the name it has in the fleet, as GET /api/v1/devices lists it. A lowercase slug. |

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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `tag` | path | string | The device's tag: the name it has in the fleet, as GET /api/v1/devices lists it. A lowercase slug. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | [DeviceWithCredential](#devicewithcredential) | OK |
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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `tag` | path | string | The device's tag: the name it has in the fleet, as GET /api/v1/devices lists it. A lowercase slug. |

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
| `200` | array of [DirectoryGroup](#directorygroup) | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `GET /api/v1/evidence`

Audit evidence export.

Self-contained JSON bundle for a period (?from=&amp;to=, RFC 3339; default last 30 days): configuration commits, change requests with authorship, rollout promotions, and the controls in force. Org-wide viewer.

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Evidence](#evidence) | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `GET /api/v1/facts/{tag}`

Hardware facts of one device.

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `tag` | path | string | The device's tag: the name it has in the fleet, as GET /api/v1/devices lists it. A lowercase slug. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | [DeviceFacts](#devicefacts) | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `PUT /api/v1/filters/{id}`

Create or replace a filter.

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `id` | path | string | The filter's id. A PUT with a new id creates the filter. A lowercase slug. |

Request body: [Filter](#filter).

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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `id` | path | string | The filter's id. A PUT with a new id creates the filter. A lowercase slug. |

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
| `200` | [Fleet](#fleet) | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/groups`

Create a group.

Request body: [NewGroup](#newgroup).

| Answer | Body | Meaning |
|---|---|---|
| `201` | [Status](#status) | Created. |
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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `name` | path | string | The group's name, as GET /api/v1/groups lists it. A lowercase slug. |

Request body: [GroupPatch](#grouppatch).

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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `name` | path | string | The group's name, as GET /api/v1/groups lists it. A lowercase slug. |

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
| `200` | [Preferences](#preferences) | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `PUT /api/v1/me/preferences`

Update my preferences.

Body: {timezone?, locale?}. Timezone must be an IANA name; locale one of the shipped UI languages.

Request body: [Preferences](#preferences).

| Answer | Body | Meaning |
|---|---|---|
| `200` | [Preferences](#preferences) | OK |
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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `id` | path | string | The policy's id. A PUT with a new id creates the policy. A lowercase slug. |

Request body: [Policy](#policy).

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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `id` | path | string | The policy's id. A PUT with a new id creates the policy. A lowercase slug. |

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
| `200` | [RolloutView](#rolloutview) | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/rollout`

Start a rollout (owner).

Request body: [RolloutStart](#rolloutstart).

| Answer | Body | Meaning |
|---|---|---|
| `201` | [RolloutState](#rolloutstate) | Created. |
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
| `200` | [RolloutState](#rolloutstate) | OK |
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

Request body: [RolloutPlanUpdate](#rolloutplanupdate).

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
| `200` | [RolloutTick](#rollouttick) | OK |
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

Request body: [NewSecretRef](#newsecretref).

| Answer | Body | Meaning |
|---|---|---|
| `201` | object with `name` | Created |
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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `name` | path | string | The secret reference's name, as GET /api/v1/secret-refs lists it. A lowercase slug. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | object with `name` | OK |
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

Request body: [SettingChange](#settingchange).

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

Request body: [SettingClear](#settingclear).

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
| `200` | array of [DeviceStatus](#devicestatus) | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `GET /api/v1/status/{tag}`

Observed status of one device.

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `tag` | path | string | The device's tag: the name it has in the fleet, as GET /api/v1/devices lists it. A lowercase slug. |

| Answer | Body | Meaning |
|---|---|---|
| `200` | [DeviceStatus](#devicestatus) | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `GET /api/v1/tokens`

List your API tokens.

| Answer | Body | Meaning |
|---|---|---|
| `200` | array of [Token](#token) | OK |
| `401` | [Error](#error) | No credential, or one that does not verify. |
| `403` | [Error](#error) | Authenticated, but no role at the required scope - or the server is read-only (--write not set), or the X-CSRF-Token is missing on a session-authenticated mutation. |
| `404` | [Error](#error) | No such endpoint, or the named resource does not exist. |
| `500` | [Error](#error) | Unexpected server error. The body is always {"error":"internal error"}; the detail is in the server log, deliberately. |

### `POST /api/v1/tokens`

Mint an API token (secret shown once).

Request body: [NewToken](#newtoken).

| Answer | Body | Meaning |
|---|---|---|
| `201` | [NewTokenAnswer](#newtokenanswer) | Created |
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

| Parameter | In | Type | Meaning |
|---|---|---|---|
| `id` | path | string | The token's id, as POST /api/v1/tokens returns it and GET /api/v1/tokens lists it. Not the secret. |

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

Required means two things. In an answer, the field is always there. In a request
body, the server needs it. The server refuses a body with a field this page does not name.

### AccessBinding

| Field | Type | Required | Meaning |
|---|---|---|---|
| `group` | string | yes | The directory group the binding grants to. |
| `role` | one of `viewer`, `editor`, `owner` | yes | The role the group gets at the scope. |
| `scope` | string | yes | Where the role holds: "org", or a group's scope such as "group:pilot". |

### AccessRevoke

Which role binding to remove.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `group` | string | yes | The identity provider group the binding is for. |
| `scope` | string | yes | Where it applies: "org", "group:&lt;name&gt;" or "device:&lt;tag&gt;". |

### AppList

One app list at one scope, replaced as a whole.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `kind` | one of `packages`, `flatpaks`, `overlays` | yes | Which list. |
| `names` | array of string | yes | The complete list after this call; an empty list clears it. |
| `scope` | string | yes | Where it applies: "org", "group:&lt;name&gt;" or "device:&lt;tag&gt;". |

### Assignment

A policy bound to a scope, optionally narrowed by a filter.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `filter` | string |  | A filter's id; the policy then applies only to the devices it selects. Empty selects every device. |
| `policy` | string | yes | The policy's id. |
| `priority` | integer |  | Breaks a tie between assignments that set the same key at the same scope: higher wins. |
| `target` | string | yes | Where it applies: "org", "group:&lt;name&gt;" or "device:&lt;tag&gt;". |

### AssignmentRemoval

Which assignment to remove.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `filter` | string |  | The filter the assignment was narrowed by, if any. |
| `policy` | string | yes | The policy's id. |
| `target` | string | yes | Where it applies: "org", "group:&lt;name&gt;" or "device:&lt;tag&gt;". |

### Assurance

The audit controls, replaced as a whole. A field left out is off.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `manualRolloutOnly` | boolean |  | Merging a change starts no rollout; somebody starts every run by hand. |
| `requireChangeRequest` | boolean |  | Settings change only through a change request, never directly. |
| `requireFourEyes` | boolean |  | A change cannot be merged by the person who made it. |
| `requireTestWave` | boolean |  | A rollout needs a first wave that waits for approval; an org owner may skip it for one rollout. |

### AuditEntry

One entry of the configuration audit trail. It is git history, so it is append-only and complete.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `author` | string |  | Who made the commit, by name. |
| `email` | string |  | Their e-mail address, as the commit records it. |
| `hash` | string |  | Commit hash. |
| `subject` | string |  | Commit subject: what changed, in the words the console wrote. |
| `when` | string |  | When it was committed. |

### ChangeEdit

One setting edit on a change request's branch.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `clear` | boolean |  | Remove the key at this scope instead of setting it. |
| `enforce` | boolean |  | Lock the value, so a narrower scope cannot weaken it. Left out, the lock stays as it is. |
| `key` | string | yes | The setting's catalog key. |
| `scope` | string | yes | Where it applies: "org", "group:&lt;name&gt;" or "device:&lt;tag&gt;". |
| `value` | any |  | The value, typed as the catalog entry for the key says: a boolean, a number, a string or a list. Ignored with clear. |

### ChangeRequest

A change request: settings edited on a branch of their own, merged into the fleet after the gate has built the devices it touches.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `author` | string | yes | Who opened it, by display name. |
| `authorSubject` | string |  | Who opened it, by the identity provider's subject; four-eyes compares this one. |
| `branch` | string | yes | The branch of the fleet repository it lives on. |
| `created` | string | yes | When it was opened. |
| `error` | string |  | Why the gate or the build refused it, when it did. |
| `hosts` | array of string |  | The devices its edits can reach, which the gate builds. |
| `id` | string | yes | Its id. |
| `status` | one of `draft`, `building`, `failed`, `ready`, `merged`, `abandoned` | yes | Where it is. |
| `title` | string | yes | What it is for. |
| `updated` | string | yes | When it last changed. |
| `wholeFleet` | boolean |  | An edit reached the whole fleet, so the gate builds every device. |

### Condition

A requirement on what a device reports, not on its configuration: it is checked, never written.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `detail` | string |  | What a failure means, in words for whoever reads the finding. |
| `metric` | string | yes | What is measured, as the device reports it, for example disk.free_percent or memory.total_mb. |
| `op` | one of `>=`, `<=`, `>`, `<`, `==` | yes | How the measurement compares to value. |
| `value` | number | yes | The threshold. |

### Device

A device as the fleet document holds it. This is the CONFIGURATION view; what a device reports about itself is /api/v1/status.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `assignedUser` | string |  | The person this device belongs to. Personal data - see docs/compliance/processing-register.md. |
| `class` | string |  | One of laptop, desktop, server, station. May be absent on a device a station discovered but nobody has classified. |
| `groups` | array of string |  | Group memberships, most general first. Always an array, never null. |
| `hardware` | string |  | Hardware profile name from the overlay. |
| `tag` | string | yes | Stable identifier. A lowercase slug; also the machine hostname. |

### DeviceDetail

One device: its record, the settings that apply to it and where each comes from, and its apps.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `apps` | object with `flatpaks`, `overlays`, `packages` | yes | The apps it gets, after every scope has added its own. |
| `device` | [DeviceRecord](#devicerecord) | yes |  |
| `resolved` | array of [ResolvedSetting](#resolvedsetting) | yes | Every setting that applies, sorted by key. |
| `tag` | string | yes | Its tag. |

### DeviceFacts

What a device last reported about its hardware.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `facts` | object | yes | The report, as the agent sent it. |
| `updatedAt` | string | yes | When it reported. |

### DeviceIntent

An action for a device to carry out at its next check-in.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `force` | boolean |  | Wipe a device that is not locked first. |
| `intent` | one of `lock`, `reboot`, `wipe`, `diagnostics` | yes | The action. |

### DevicePatch

The device fields to change. A field left out stays as it is.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `assignedUser` | string |  | Who uses the device. |
| `class` | string |  | The device class, such as laptop. |
| `groups` | array of string |  | The groups the device is in, as a complete list. |
| `hardware` | string |  | The hardware profile the device's configuration is built for. |
| `itam` | [ITAM](#itam) |  |  |
| `labels` | object of string values |  | Free key-value labels, as a complete set. |

### DeviceRecord

A device as the fleet repository holds it.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `accepted` | object of string values |  | Accepted risks: a control's key to why it does not hold here. |
| `assignedUser` | string |  | Who uses it. |
| `class` | string |  | The device class, such as laptop. |
| `enforced` | array of string |  | Keys among settings that cannot be changed for it elsewhere. |
| `enrolled` | string |  | When the record was made. |
| `flatpaks` | array of string |  | Flatpak apps for this device itself. |
| `groups` | array of string |  | The groups it is in. Order counts: where two unrelated groups set the same key, the earlier one wins. |
| `hardware` | string | yes | The hardware profile its configuration is built for. |
| `intent` | string |  | An action waiting for its next check-in. Empty is none. |
| `itam` | [ITAM](#itam) |  |  |
| `labels` | object of string values |  | Free key-value labels. |
| `overlays` | array of string |  | Overlay modules for this device itself. |
| `packages` | array of string |  | Nix packages for this device itself. |
| `pin` | string |  | A rollout wave's group this device follows before the rest of its group does. |
| `settings` | object of any values |  | Settings set on this device itself. |
| `spec` | [HardwareSpec](#hardwarespec) |  |  |
| `state` | one of ``, `provisional`, `retired` |  | Where it is in its life. Empty is active. |

### DeviceStatus

What a device last reported about itself.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `ack` | string |  | The last action the device confirmed carrying out. |
| `error` | string |  | What went wrong, when the device said something did. |
| `health` | [Health](#health) |  |  |
| `integrations` | object of [IntegrationState](#integrationstate) values |  | Each integration turned on for it, by name, as it last saw it. |
| `lastSeen` | string | yes | When it last checked in. |
| `online` | boolean | yes | It checked in recently enough to count as there. |
| `phase` | one of `discovered`, `installing`, `installed`, `running` |  | Where it is in its installation. |
| `revision` | string |  | The fleet revision it runs. |
| `sb` | string |  | Secure Boot, as the device sees it. Empty when it did not say. |
| `tag` | string | yes | Its tag. |
| `tpm2` | string |  | The TPM2, as the device sees it. Empty when it did not say. |
| `usage` | [Usage](#usage) |  |  |

### DeviceWithCredential

A device that may check in again, with the credential to do it.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `credential` | string |  | The secret the device checks in with. Shown once; store it before leaving this answer. |
| `credentialError` | string |  | Present when no credential could be made: make one with POST /api/v1/devices/{tag}/credential before the device checks in. |
| `notice` | string |  | A reminder that the credential is not shown again. |
| `status` | string | yes | What happened. |
| `tag` | string |  | The device's tag. |

### DirectoryGroup

A group in the identity provider's directory.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `id` | string | yes | Its id in the directory. |
| `name` | string | yes | Its name. |

### Error

| Field | Type | Required | Meaning |
|---|---|---|---|
| `error` | string | yes | Human-readable reason. Safe to show an operator: it never carries internals - a 500 is always the literal string "internal error", with the detail in the server log. |

### Evidence

What an auditor asks for over a period: the controls in force, and every commit, change request and rollout in it.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `changes` | array of [ChangeRequest](#changerequest) | yes | Every change request touched in the period. |
| `commits` | array of [AuditEntry](#auditentry) | yes | Every commit to the fleet repository in the period, newest first. |
| `controls` | [EvidenceControls](#evidencecontrols) | yes |  |
| `from` | string | yes | The start of the period, included. |
| `generatedAt` | string | yes | When this was made. |
| `rollouts` | array of [EvidenceRollout](#evidencerollout) | yes | Every promotion of a rollout wave in the period. |
| `to` | string | yes | Its end, not included. |

### EvidenceControls

The audit controls in force when the evidence was made.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `manualRolloutOnly` | boolean | yes | Merging starts no rollout. |
| `requireChangeRequest` | boolean | yes | Settings change only through a change request. |
| `requireFourEyes` | boolean | yes | A change cannot be merged by the person who made it. |
| `requireTestWave` | boolean | yes | A rollout needs a first wave that waits for approval. |

### EvidenceRollout

One rollout step.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `hash` | string | yes | Its hash. |
| `subject` | string | yes | The commit that moved the wave. |
| `when` | string | yes | When. |

### Filter

A test on device attributes that selects devices.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `match` | one of `all`, `any` |  | Whether every rule must hold or one is enough. Left out, every rule. |
| `name` | string |  | A label for people; the id in the path is what refers to it. |
| `rules` | array of [FilterRule](#filterrule) | yes | The rules. |

### FilterRule

One test on one device attribute.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `attr` | string | yes | The attribute: tag, class, hardware, assignedUser, group (membership, including parent groups) or "label:&lt;key&gt;". |
| `op` | one of `eq`, `ne`, `prefix`, `in` | yes | How it is compared. |
| `value` | string |  | The value to compare with; for every op but in. |
| `values` | array of string |  | The values to compare with; for in. |

### Fleet

The fleet document as the fleet repository holds it in fleet.json, with only what the caller may see.

Type: object.

### GroupPatch

The group fields to change; at least one. A field left out stays as it is.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `idpGroup` | string |  | The identity provider group whose members belong to it. |
| `parent` | string |  | The new parent group; empty makes it a top-level group. |

### HardwareSpec

The hardware an installation station found when it enrolled the device.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `cores` | integer |  | Processor cores. |
| `cpu` | string |  | The processor. |
| `diskGB` | integer |  | Disk, in GB. |
| `firmware` | string |  | The firmware version. |
| `memGB` | integer |  | Memory, in GB. |
| `model` | string |  | The model. |
| `serial` | string |  | The serial number. |
| `vendor` | string |  | The maker. |

### Health

systemd's last verdict on the machine.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `failedUnits` | array of string |  | The units that failed. |
| `state` | string |  | systemd's own word, such as "running" or "degraded". |

### HostKey

| Field | Type | Required | Meaning |
|---|---|---|---|
| `hostKey` | string | yes | The device's SSH host public key, as recorded at enrolment. |
| `tag` | string | yes | The device's tag. |

### ITAM

Asset-management details of a device.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `hostKeyId` | string |  | The id of the device's SSH host key. |
| `model` | string |  | The model. |
| `owner` | string |  | Who owns the asset. |
| `serial` | string |  | The serial number. |
| `site` | string |  | Where it is. |

### IntegrationState

One integration as the device last saw it.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `detail` | string |  | What the device saw, such as the address it reached. |
| `state` | string |  | "up" or "down"; empty when nobody measured, which is not the same as down. |

### IntentArmed

An action armed for a device.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `intent` | string | yes | The action. |
| `status` | string | yes | What happened. |

### IssuedCredential

A device credential, shown this once.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `credential` | string | yes | The secret the device checks in with. The console keeps only its hash. |
| `notice` | string | yes | A reminder that this is the only time it is shown. |

### Me

Who the caller is and what they may do. The roles map is resolved per request, never stored.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `ceiling` | string |  | Present only for a scoped token: the highest role it may reach regardless of the underlying grant. |
| `email` | string |  | E-mail address from the identity provider. |
| `groups` | array of string |  | Directory groups. Always an array, never null. |
| `name` | string |  | Display name from the identity provider. |
| `roles` | object of string values |  | Resolved role per scope, e.g. {"org":"editor","group:pilot":"owner"}. Only scopes where the caller holds at least viewer appear. |
| `service` | boolean |  | True for a service account rather than a person. |
| `subject` | string |  | Identifier from the identity provider. |

### NewChange

A change request to open.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `id` | string | yes | The id to give it. A lowercase slug. |
| `title` | string | yes | What the change is for. |

### NewDevice

A device to enroll.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `assignedUser` | string |  | Who uses it. |
| `class` | string |  | The device class, such as laptop. |
| `groups` | array of string |  | The groups it is in. |
| `hardware` | string | yes | The hardware profile its configuration is built for. |
| `labels` | object of string values |  | Free key-value labels. |
| `tag` | string | yes | Its name in the fleet. A lowercase slug. |

### NewGroup

A group to create.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `idpGroup` | string |  | The identity provider group whose members belong to it. |
| `name` | string | yes | Its name. A lowercase slug. |
| `parent` | string |  | The group it sits in; left out, it is a top-level group. |

### NewSecretRef

A secret reference to register: the name of a secret the devices get from elsewhere, never the secret.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `description` | string |  | What the secret is for. |
| `name` | string | yes | The name. A lowercase slug. |

### NewToken

An API token to make.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `ceiling` | one of `viewer`, `editor`, `owner` |  | The highest role the token may use, whatever its holder has. |
| `groups` | array of string |  | For a service account: the groups it is in. |
| `id` | string | yes | Its id, by which it is listed and revoked. A service account becomes the principal svc:&lt;id&gt;. |
| `kind` | one of `personal`, `service` |  | A personal token acts as you; a service account is a principal of its own, made by an org owner. Left out, personal. |
| `name` | string | yes | A label for people. |
| `ttlDays` | integer |  | How many days it is valid. Left out, the console's default. |

### NewTokenAnswer

A token, with its secret shown this once.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `notice` | string | yes | A reminder that the secret is not shown again. |
| `secret` | string | yes | The bearer secret. Shown once; the console keeps only its hash. |
| `token` | [Token](#token) | yes |  |

### Policy

A named set of settings and conditions, applied where it is assigned.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `conditions` | array of [Condition](#condition) |  | Requirements on what a device reports. |
| `controls` | array of string |  | The compliance controls it implements, such as "BIO 12.3.1". Free text. |
| `description` | string |  | What the policy is for. |
| `enforced` | array of string |  | The keys among settings that a narrower scope cannot weaken. |
| `name` | string |  | A label for people; the id in the path is what refers to it. |
| `profile` | string |  | The overlay profile the policy was made from, as name@hash. Empty for a policy made by hand. |
| `settings` | object of any values | yes | Setting keys from the catalog, with their values. |

### Preferences

Your own preferences. Empty means the organisation's default.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `locale` | one of `en`, `nl` |  | The console's language. |
| `timezone` | string |  | An IANA time zone, such as "Europe/Amsterdam". |

### ResolvedSetting

One setting as it applies to a device, and where it comes from.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `enforced` | boolean | yes | A narrower scope cannot change it. |
| `key` | string | yes | The setting's catalog key. |
| `source` | [SettingSource](#settingsource) | yes |  |
| `value` | any | yes | Its value. |

### RingProgress

How far one wave of the active run is. The field names start with a capital, unlike the rest of the API.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `Absent` | integer | yes | Devices not seen for a while; they do not count against the wave. |
| `Broken` | integer | yes | Of those on target, how many report an error or are stuck. |
| `GroupTotal` | integer | yes | How many devices the group has. |
| `Healthy` | integer | yes | Of those on target, how many are healthy. |
| `OnTarget` | integer | yes | Of those that are there, how many run the target. |
| `Released` | integer | yes | How many of the group have been given the target so far. |
| `Total` | integer | yes | Devices in the wave so far. |

### RolloutPlan

The waves a change rolls out in.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `autoFlow` | boolean |  | Start a rollout by itself when a change reaches the fleet repository. Left out, on. |
| `rings` | array of [RolloutRing](#rolloutring) |  | The waves, in order. |

### RolloutPlanUpdate

The rollout plan to keep.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `plan` | [RolloutPlan](#rolloutplan) | yes |  |

### RolloutRing

One wave of a rollout.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `group` | string |  | The device group the wave covers; set this or groups. |
| `groups` | array of string |  | The device groups the wave covers, when it is more than one. |
| `maxDevices` | integer |  | At most this many devices get the change at once. 0 is the whole group. |
| `minDevices` | integer |  | How many devices must report before the percentage counts. 0 is half the wave. |
| `minHealthyPercent` | integer |  | How many of its devices, in percent, must be healthy to go on. |
| `name` | string |  | A label, such as Canary or Test. |
| `requireApproval` | boolean |  | Wait for somebody to approve before the next wave. |
| `soakMinutes` | integer |  | How long the wave runs before the next may start. |

### RolloutStart

The revision to roll out.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `target` | string | yes | A commit of the fleet repository. |

### RolloutState

A rollout run.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `approvedAt` | object of string values |  | Per wave: when somebody approved it. |
| `buildRequestedAt` | object of string values |  | Per wave: when its build was asked for. |
| `changeId` | string |  | The change request the run delivers, when it delivers one. |
| `changeTitle` | string |  | That change request's title. |
| `convergedAt` | object of string values |  | Per wave: when all its devices first ran the target. |
| `promotedAt` | object of string values |  | Per wave, by number: when it was given the target. |
| `reason` | string |  | Why it halted. |
| `ring` | integer | yes | The wave it is on, counted from 0. |
| `rings` | array of [RolloutRing](#rolloutring) |  | The run's own waves, fixed when it started. |
| `started` | string | yes | When it started. |
| `status` | one of `active`, `paused`, `halted`, `completed`, `cancelled` | yes | Where the run is. |
| `target` | string | yes | The revision every wave moves to. |
| `updated` | string | yes | When it last changed. |
| `waiting` | string |  | What it waits for, in words; empty once it has ended. |
| `waitingSince` | string |  | Since when it has waited for that. |

### RolloutTick

What one step of the rollout engine did.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `action` | object with `kind`, `reason` |  | What the step decided; absent when no run is active. |
| `state` | object | yes | The run's state after the step, as GET /api/v1/rollout answers it; null when there has never been a run. |

### RolloutView

The rollout as it stands.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `active` | boolean | yes | A run is going. |
| `rings` | array of [RingProgress](#ringprogress) |  | Progress per wave of the run. |
| `state` | [RolloutState](#rolloutstate) |  |  |

### SecretRef

| Field | Type | Required | Meaning |
|---|---|---|---|
| `description` | string |  | What the secret is for. |
| `name` | string | yes | The reference a setting uses. Never a value. |

### SettingChange

One setting at one scope.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `enforce` | boolean |  | Lock the value, so a narrower scope cannot weaken it. Left out, the lock stays as it is. |
| `key` | string | yes | The setting's catalog key. |
| `scope` | string | yes | Where it applies: "org", "group:&lt;name&gt;" or "device:&lt;tag&gt;". |
| `value` | any | yes | The value, typed as the catalog entry for the key says: a boolean, a number, a string or a list. |

### SettingClear

Which setting to remove.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `key` | string | yes | The setting's catalog key. |
| `scope` | string | yes | Where it applies: "org", "group:&lt;name&gt;" or "device:&lt;tag&gt;". |

### SettingSource

Where a setting comes from.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `policy` | string |  | The policy that sets it there, when a policy does. |
| `scope` | string | yes | The scope that sets it: "org", "group:&lt;name&gt;" or "device:&lt;tag&gt;". |

### Status

The answer to a write that has nothing to return but its own success.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `status` | string | yes | What happened, e.g. "enrolled", "removed", "applied". |

### Token

An API token, without its secret.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `ceiling` | string |  | The highest role it may use. Empty is no limit beyond its holder's. |
| `created` | string | yes | When it was made. |
| `expires` | string | yes | When it stops working. |
| `groups` | array of string |  | The groups it carries. |
| `id` | string | yes | Its id. |
| `kind` | one of `personal`, `service`, `device`, `station` | yes | What it is. |
| `lastUsed` | string |  | When it was last used. |
| `name` | string | yes | Its label. |
| `subject` | string | yes | Who it acts as. |

### Usage

What the device last measured of its own load.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `cpuPct` | integer |  | Processor use, in percent. |
| `diskTotalGB` | integer |  | Disk, in GB. |
| `diskUsedGB` | integer |  | Disk in use, in GB. |
| `memTotalMB` | integer |  | Memory, in MB. |
| `memUsedMB` | integer |  | Memory in use, in MB. |
