# Session search in the minimization fork

Use the search palette to search conversations with SQLite FTS5. Results show
matching snippets and link to the session. Project/date filters and relevance
or recency sorting remain available. Search needs no model or external service.

Embedding generation, semantic/hybrid search controls, and the embeddings
settings panel are removed. Saved browser mode preferences fall back to FTS.
Existing vector files are not deleted. Legacy semantic API requests report
unavailable; use lexical search instead.

Open a session and choose Show session analysis to add or remove tags and edit
its summary. Save summary stores the text locally; it does not invoke a model.
The search palette also matches saved summaries. Click a tag to filter sessions.
These annotations survive transcript reimports. Automatic approved-harness
summary generation is not connected to this panel yet.
