# Link Check

A Go command-line utility for finding missing local links in Markdown files. It can also check HTTP and HTTPS links when asked.

## Requirements

Go 1.21 or newer. No third-party packages.

## Run

~~~sh
go run . ./sample-docs
go run . -remote ./sample-docs
~~~

The first command checks relative file links. The second also sends HEAD requests to web links, with a short timeout. It exits with a nonzero status if a broken link is found.

Checks Markdown inline links and images. It verifies local target files, but does not validate heading anchors.
