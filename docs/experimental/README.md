# Experimental Docs

This folder contains future-facing documents that are still strategically relevant, but are not part of the active product contract.

Use this folder for:
1. future capability specs
2. long-range architectural directions
3. moat candidates not yet in the active runtime

Do not treat these documents as current rollout doctrine.

## Status Map

### Active candidates
1. `agentic-decision-plane-spec.md`
2. `discovery-memory-spec.md`

### Partially absorbed by the runtime
1. `discovery-memory-spec.md`

Reason:
1. local discovery state already exists
2. provider health memory already exists
3. seedless local reuse is already part of the product
4. the document still matters because the full memory substrate is not complete

### Moved to archive
1. `seedless-discovery-strategy.md` -> [`../archive/seedless-discovery-strategy.md`](../archive/seedless-discovery-strategy.md)

Reason:
1. its framing of seedless as merely `best-effort` is no longer aligned with the product direction
2. parts of its provider/taxonomy doctrine are already absorbed
3. it is still useful as historical strategy context, not as current experimental doctrine
