# Search implementation in the minimization fork

Conversation search uses SQLite FTS5 indexes owned by `internal/db`. The UI
search palette calls the local search API and renders its ranked snippets.

The vector engine, embedding schedulers and encoders, build commands, and
embedding API routes have been removed. Legacy semantic API types remain for
compatibility, but no production semantic searcher is installed. These types
do not load sqlite-vec or invoke an encoder.
