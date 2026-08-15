# Xion Owner Quota Design

## Goal

Add an opt-in logical byte quota for each `metadata.owner`, separate from filesystem pause protection. The quota applies to active files plus files retained in the trash, so soft-delete cannot bypass the limit.

## Configuration and API

```text
XION_OWNER_QUOTA_BYTES=10737418240
GET /api/v1/files/quota?owner=blog
```

`XION_OWNER_QUOTA_BYTES=0` disables owner quotas. Missing or blank owners use `_anonymous`. The quota endpoint requires the service token and returns limit, used, available, and percentage.

## Enforcement

- One-shot uploads check the owner quota after staging and before publishing the object.
- Resumable sessions reserve their declared size when created; a session larger than the remaining quota is rejected before writing chunks.
- Soft-deleted objects remain counted until permanently cleaned by a future retention job.
- Exceeding quota returns HTTP `507` with `quota_exceeded`; reads, downloads, and deletes remain available.

## Compatibility

The default quota is disabled, existing upload contracts remain valid, and stores created by tests without quota options behave as before.
