# Reference regeneration from character nodes

Add a Reference images action and current preview to character nodes. The action selects the node and focuses its reference section in the existing details panel; generation is an explicit button there, with provider-charge copy, pending review status and previous versions. Save/discard edited profile fields first. Background work survives leaving the panel; late callbacks cannot populate another character's panel.

Use the shared reference-sheet API, including Library generation. Regeneration must accept rejected/pending drafts without conditioning images on them: an internal-only purpose flag filters the character generation snapshot to approved available references. Ordinary scene/video approval rules remain unchanged, and JSON clients cannot set the private flag. After successful generation, atomically retain approved active bindings and add the new pending candidate, retiring superseded drafts from bindings while keeping all assets in Library. Failure leaves bindings untouched. Concurrent manual requests for one character are rejected while automatic missing-sheet requests can wait/reuse. Enforce active-project ownership.

Graph exposes current reference bindings/review state; only active edges are reference edges. Layout reserves thumbnail/action height. Test rejected-draft repair, failed generation, ownership, binding/history preservation, current graph previews, background node switching, draft protection, keyboard/mobile accessibility and the existing automatic story pipeline. No live-paid generation.

Observer cleanup also guards callbacks already queued when the reference component is destroyed. Closing/switching views stops local tracking without cancelling paid work on the server.
