# Candidate Intelligence Roadmap

## Goal
Strengthen seedless and technical discovery using semantic classification and clustering without turning the core into a keyword rule engine.

## Current Constraints
- The shared discovery path is still dominated by provider noise and ranking misses.
- Structural signals are useful, but not sufficient on their own for ambiguous first-party vs third-party choices.
- Broad LLM usage is too expensive and too unstable for the first ranking pass.

## Direction
Introduce a candidate intelligence layer between bootstrap retrieval and final selection.

Pipeline shape:
1. bootstrap retrieval
2. candidate annotation
3. semantic classification / clustering
4. family-aware rerank
5. narrow LLM extraction only when ambiguity remains

## Candidate Annotation
Each candidate should carry compact machine-usable metadata:
- `resource_class`
- `host_family`
- `host_root_title`
- `page_title`
- `semantic_family_score`
- `cluster_id`
- `cluster_confidence`
- `selection_risk`

This layer should annotate, not hard-drop, except for explicit invalid URLs.

## Candidate Classification
Target classes:
- `first_party_docs`
- `first_party_home`
- `third_party_tutorial`
- `third_party_wrapper`
- `reference_index`
- `asset_file`
- `structured_endpoint`
- `unknown`

Implementation options, in ascending cost:
1. embedding + centroid scoring
2. embedding + lightweight classifier
3. graph classification on small candidate sets

## Candidate Clustering
For small candidate sets, the practical options are:
- agglomerative clustering on embedding distance
- graph clustering with edge weights from:
  - semantic similarity
  - same registrable domain
  - same host root identity
  - same-page literal URL provenance

This is especially useful for collapsing families like:
- first-party docs subtree
- third-party tutorial subtree
- mirror / wrapper subtree

## Recommended Near-Term Path
1. keep the core retrieval path deterministic
2. add candidate annotations first
3. add family-level semantic clustering on the top N candidates
4. expose cluster metadata in traces and benchmark reports
5. use no-thinking micro models only for final disambiguation or endpoint extraction

## What Not To Do
- no global keyword lists for intent routing
- no full LLM ranking over all candidates
- no brittle provider-specific hacks in the core scoring path

## Expected Benefits
If implemented well, this should improve:
- seedless first-party selection
- endpoint/reference retrieval
- ambiguity handling on noisy SERPs
- traceability of why a candidate won
