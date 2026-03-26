# Release Notes

Create one Markdown file per tag in this directory.

Naming convention:

- `release-notes/v0.1.0.md`
- `release-notes/v0.1.1.md`

When a tag matching `v*` is pushed, the release workflow will read `release-notes/<tag>.md` and use that file as the GitHub Release body.

If the file is missing, the workflow will fail so the release is not published with an empty description.
