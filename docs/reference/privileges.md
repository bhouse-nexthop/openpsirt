# Privileges

Who may call what. **Generated from the API document**, which is generated from
the server — so this cannot describe a rule the code does not enforce, or miss
an endpoint it serves.

Access is granted in advance or not at all: no account is created by signing
in. A role is held per product except where noted, and being able to *see*
something is never the same as being able to change it.

## The roles

| Role | What it allows |
|---|---|
| `public-read` | Read findings that have been disclosed |
| `private-read` | Read findings nobody has announced |
| `public-triage` | Argue about disclosed findings: decide, revise, withdraw, comment. Take work nobody owns, and hand back your own |
| `private-triage` | The same for findings nobody has announced, including recording one and moving its disclosure date |
| `approver` | Agree to somebody else's claim. A triager may also approve, and the proposer may never approve their own |
| `assigner` | Give work to somebody else, or take what they are holding |
| `reporting` | Read the reports for a product |
| administrator | The deployment: people, roles, credentials, settings, and the catalog |

Two rules are not roles and cannot be granted. **Visibility narrows every
answer**, so an endpoint you may call still shows only what you may see. And
**the proposer of a claim may never approve it**, whoever they are.

## Administrator

Held for the deployment rather than per product. Holding every product role there is does not amount to any of it.

| | Endpoint | Roles | |
|---|---|---|---|
| DELETE | `/v1/attachments/{token}` | — |  |
| GET | `/v1/keys` | — |  |
| POST | `/v1/keys` | — |  |
| DELETE | `/v1/keys/{name}` | — |  |
| GET | `/v1/people` | — |  |
| POST | `/v1/people` | — |  |
| GET | `/v1/people/tokens` | — |  |
| POST | `/v1/people/{identity}/assignments/release` | — |  |
| DELETE | `/v1/people/{identity}/roles/{product}/{role}` | — |  |
| DELETE | `/v1/people/{identity}/sessions` | — |  |
| DELETE | `/v1/people/{identity}/tokens/{name}` | — |  |
| POST | `/v1/products` | — |  |
| PUT | `/v1/products/{product}/end-of-life` | — |  |
| POST | `/v1/products/{product}/streams` | — |  |
| PUT | `/v1/products/{product}/streams/{stream}/end-of-life` | — |  |
| PUT | `/v1/products/{product}/triage-floor` | — |  |
| POST | `/v1/products/{product}/variants` | — |  |
| DELETE | `/v1/roles/bindings` | — |  |
| GET | `/v1/roles/bindings` | — |  |
| POST | `/v1/roles/bindings` | — |  |
| GET | `/v1/roles/mode` | — |  |
| PUT | `/v1/roles/mode` | — |  |
| GET | `/v1/settings` | — |  |
| PUT | `/v1/settings/{name}` | — |  |

## A role on the product

Granted per product. Any one of the roles listed is enough — a dash means any role on that product will do — and what you may see narrows what the answer contains.

| | Endpoint | Roles | |
|---|---|---|---|
| DELETE | `/v1/approval-batches/{batch}` | approver, public-triage, private-triage |  |
| POST | `/v1/claims/{id}/approval` | approver, public-triage, private-triage | The proposer may not approve their own. |
| POST | `/v1/claims/{id}/send-back` | approver, public-triage, private-triage | The proposer may not approve their own. |
| PUT | `/v1/comments/{id}` | approver, public-triage, private-triage | Only the author may edit a comment. |
| DELETE | `/v1/decisions/{id}` | public-triage, private-triage |  |
| POST | `/v1/decisions/{id}/approval` | approver, public-triage, private-triage | The proposer may not approve their own. |
| POST | `/v1/decisions/{id}/comments` | approver, public-triage, private-triage |  |
| PUT | `/v1/decisions/{id}/reasoning` | public-triage, private-triage |  |
| POST | `/v1/decisions/{id}/send-back` | approver, public-triage, private-triage | The proposer may not approve their own. |
| GET | `/v1/disclosing` | private-read, private-triage | Only where you may read undisclosed work. |
| POST | `/v1/disclosure-extensions/{id}/approval` | private-triage | Not the person who asked for it. |
| GET | `/v1/products/{product}/issues/{vulnerability}/disclosure` | private-read, private-triage | Only where you may read undisclosed work. |
| POST | `/v1/products/{product}/issues/{vulnerability}/disclosure` | private-triage | A second person agrees past the threshold. |
| GET | `/v1/products/{product}/mentionable` | — | Asking about undisclosed findings needs private-read or private-triage. |
| GET | `/v1/products/{product}/streams/{stream}/variants/{variant}/carried` | public-triage, private-triage |  |
| POST | `/v1/products/{product}/streams/{stream}/variants/{variant}/carried` | public-triage, private-triage |  |
| POST | `/v1/products/{product}/streams/{stream}/variants/{variant}/components/{component}/decisions` | public-triage, private-triage |  |
| POST | `/v1/products/{product}/streams/{stream}/variants/{variant}/findings` | public-triage, private-triage | private-triage where the finding is undisclosed. |
| PUT | `/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}/assignment` | public-triage, private-triage | Giving work to somebody else also needs assigner. Taking unowned work, or handing back your own, does not. |
| POST | `/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}/decision` | public-triage, private-triage |  |
| PUT | `/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}/fix-targets` | public-triage, private-triage |  |
| POST | `/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/places/{place}/decision` | public-triage, private-triage |  |
| POST | `/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/places/{place}/decision/reaffirmation` | public-triage, private-triage |  |
| POST | `/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/resolve` | public-triage, private-triage |  |
| POST | `/v1/products/{product}/streams/{stream}/variants/{variant}/scans` | public-triage, private-triage | A pipeline key covering this product, branch and variant sends without any of these, and is how a build sends. |

## A role on any product

Not about a product at all: a rating is a claim about an issue, true wherever it appears, so there is no product to hold a role on. The role is asked for anywhere instead, because being signed in is not a role.

| | Endpoint | Roles | |
|---|---|---|---|
| DELETE | `/v1/assessments/{id}` | public-triage, private-triage |  |
| POST | `/v1/assessments/{id}/agreement` | approver, public-triage, private-triage | The proposer may not approve their own. |
| POST | `/v1/issues/{vulnerability}/assessment` | public-triage, private-triage |  |

## Your own

About whoever is asking. No role is needed and none helps.

| | Endpoint | Roles | |
|---|---|---|---|
| DELETE | `/v1/notifications` | — |  |
| GET | `/v1/notifications` | — |  |
| DELETE | `/v1/notifications/{id}` | — |  |
| DELETE | `/v1/session` | — |  |
| GET | `/v1/session/me` | — |  |
| PUT | `/v1/session/me/digest` | — |  |
| GET | `/v1/tokens` | — |  |
| POST | `/v1/tokens` | — | Signed in, not through a token: a token cannot mint another. |
| DELETE | `/v1/tokens/{name}` | — | Signed in, not through a token: a token cannot withdraw another. |

## Any credential

Any credential this deployment recognizes. The answer is narrowed to what you may see, so two people calling the same endpoint get different answers rather than one of them getting an error.

| | Endpoint | Roles | |
|---|---|---|---|
| GET | `/v1/assessments` | — | Every claim, whoever asks: a rating is about an issue, not a product (TRI-40), so there is nothing here to narrow by. |
| GET | `/v1/assignments` | — | Answers only what you may see. |
| GET | `/v1/attachments/{token}` | — | Answers only what you may see. |
| GET | `/v1/audit` | — | Answers only what you may see. |
| GET | `/v1/decisions` | — | Answers only what you may see. |
| GET | `/v1/decisions/{id}` | — | Answers only what you may see. |
| GET | `/v1/decisions/{id}/approvals` | — | Answers only what you may see. |
| GET | `/v1/decisions/{id}/comments` | — | Answers only what you may see. |
| GET | `/v1/decisions/{id}/revisions` | — | Answers only what you may see. |
| GET | `/v1/deferrals/repeated` | — | Answers only what you may see. |
| GET | `/v1/people/{identity}/assignments` | — | Answers only what you may see. |
| GET | `/v1/products` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/comparison` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/comparison/notes` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/findings` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/findings/components` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/issues/{vulnerability}/advisory` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/issues/{vulnerability}/attachments` | — | Answers only what you may see. |
| POST | `/v1/products/{product}/issues/{vulnerability}/attachments` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/releases` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/streams` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/streams/{stream}/variants` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/streams/{stream}/variants/{variant}/components` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/streams/{stream}/variants/{variant}/components/{component}/around` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/streams/{stream}/variants/{variant}/components/{component}/issues` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}/fix-targets` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/places/{place}/decision` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/places/{place}/reach` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/streams/{stream}/variants/{variant}/readiness` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/streams/{stream}/variants/{variant}/scans` | — | Answers only what you may see. |
| GET | `/v1/products/{product}/variants` | — | Answers only what you may see. |
| GET | `/v1/remediation` | — | Answers only what you may see. |
| GET | `/v1/review-queue` | — | Answers only what you may see. |
| GET | `/v1/running-out` | — | Answers only what you may see. |
| GET | `/v1/scanning` | — | Answers only what you may see. |
| GET | `/v1/score` | — | Answers a calculation, and reads nothing. |
| GET | `/v1/sign-in` | — | Answered without a credential. |
| GET | `/v1/trend` | — | Answers only what you may see. |
| GET | `/v1/trend/releases` | — | Answers only what you may see. |
| GET | `/v1/unassigned` | — | Answers only what you may see. |
| GET | `/v1/version` | — | Answers only what you may see. |
