> **Archived.** This library was folded into the internal packages of [open-cli-collective/google-cli](https://github.com/open-cli-collective/google-cli), which builds both `gro` and `grw` from one module. Nothing depends on this module any more.

# google-cli-common

Shared Go library for the Open CLI Collective's Google CLIs. It holds everything
that would otherwise be duplicated between them: the Google OAuth flow, the API
service clients (Gmail, Calendar, Contacts, Drive, People), credential/config/
cache state, rendering and bulk-operation helpers, and the cobra command
scaffolding (init, credential management, the `mail` command surface, root
wiring).

Each CLI is a thin binary that registers a `config.Identity` at startup and
composes the pieces it needs:

- [`google-readonly`](https://github.com/open-cli-collective/google-readonly)
  (`gro`) — non-destructive: read plus labeling/archiving/RSVP/etc.
- [`google-readwrite`](https://github.com/open-cli-collective/google-readwrite)
  (`grw`) — read-write Gmail cleanup: delete, filters, and label (folder)
  lifecycle.

## The identity seam

One shared library backs multiple CLIs because everything CLI-specific is
funneled through a single `config.Identity`, registered once from `main` before
any other call:

```go
config.Register(config.Identity{
    DirName:           "google-readwrite",        // config/cache dir, keyring service, env-var prefix
    DefaultRef:        "google-readwrite/default", // default <service>/<profile> credential ref
    ProductName:       "grw",                      // user-facing name in messages
    Scopes:            appidentity.Scopes,         // OAuth scopes this CLI requests
    ScopeDescriptions: appidentity.ScopeDescriptions,
    SiblingDirNames:   []string{"google-readonly"}, // OAuth clients this CLI may reuse
})
```

`DirName` alone drives the config/cache directory, the keyring service segment,
and the derived `<SERVICE>_KEYRING_BACKEND` / `<SERVICE>_KEYRING_PASSPHRASE` /
`<SERVICE>_CREDENTIAL_REF` env vars, so two CLIs never collide.

## Layout

| Package | Responsibility |
| --- | --- |
| `config`, `keychain`, `auth` | Config file, OS-keyring token storage, and the Google OAuth flow |
| `gmail`, `calendar`, `contacts`, `drive`, `people` | API clients and data models |
| `mailcmd`, `initcmd`, `configcmd`, `setcred`, `refreshcmd`, `profilescmd`, `rootutil` | Shared cobra command packages and root scaffolding |
| `bulk`, `output`, `format`, `view`, `errors`, `log`, `cache`, `identitycache`, `zip`, `version` | Rendering, bulk-operation, and support utilities |
| `testutil`, `credtest` | Test fixtures, assertions, and a hermetic credential environment |

## Development

```bash
make check   # tidy + lint + test (race) + build
```

Secret/state handling, output conventions, and repository layout follow the
Open CLI Collective standards in
[`cli-common`](https://github.com/open-cli-collective/cli-common/tree/main/docs).
