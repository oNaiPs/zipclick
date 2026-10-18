# bodgit/sevenzip v1.6.5, patched

A copy of github.com/bodgit/sevenzip v1.6.5 (BSD 3-Clause, see LICENSE),
used through a `replace` in ZipClick's go.mod, with one change in
`types.go`, marked "ZipClick patch":

- `readFilesInfo` refuses a header declaring more than 4,194,304 files. The
  count sized a single allocation, so a 41-byte .7z declaring 0xFFFFFFFF
  files asked for 479 GiB and the process died with an unrecoverable
  out-of-memory error.

Two panics the same review found on crafted headers (a nil streamsInfo in
`reader.go`, an unchecked digest index in `struct.go`) are left as they are:
ZipClick's `Extract` recovers them and reports a damaged archive.

Drop this copy and the `replace` once upstream bounds the file count.
