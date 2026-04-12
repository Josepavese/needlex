# Topic-Root Memory Spec

Status: experimental candidate  
Updated: 2026-04-12

## Thesis

Needle-X should stop treating local discovery memory as a flat set of page embeddings and move toward a two-level retrieval substrate:

1. observed pages
2. inferred topic roots

The target problem is already visible in warm-state failures on dense documentation families such as MDN:

- the system finds the correct host family
- the system finds semantically related pages
- but it converges to a semantically dense leaf like `AsyncGenerator` instead of the topic root like `/Web/JavaScript`

This is not a fetch problem.
It is not a same-site recovery problem.
It is a representation problem.

The retrieval unit is wrong.

## Core Claim

For topic-level and overview-style queries, page-level nearest-neighbor retrieval is structurally insufficient on dense documentation families.

What is needed is a retrieval layer that can represent:

1. topic roots
2. intermediate topic nodes
3. leaf pages

and decide which granularity to return.

## Why The Current Model Fails

Dense page retrieval tends to reward:

1. semantic density
2. specificity
3. local lexical/semantic concentration

This favors leaves over roots.

On MDN, a leaf page can be a stronger embedding neighbor than the broader topic page because:

1. it contains more concentrated semantic content
2. it is more "about something"
3. it often dominates cosine similarity even when it is granularity-wrong

The result is a family-correct but topic-wrong retrieval.

## Proposed Architecture

### 1. Topic Nodes

Persist synthetic topic nodes in discovery memory.

Each topic node should include:

1. `host`
2. `root_path`
3. `language`
4. `support_count`
5. `child_count`
6. `semantic_summary`
7. `embedding`
8. `representative_page_url`
9. `topic_depth`
10. `observed_at / updated_at`

Topic nodes are not scraped pages.
They are inferred semantic aggregates built from observed descendants.

### 2. Topic-First Retrieval

Retrieval order for local-first discovery should become:

1. topic nodes
2. then pages under the winning topic
3. then same-site expansion
4. only then public bootstrap

This changes the question from:

- "which page is nearest to the query?"

to:

- "which topic manifold best explains the query?"

### 3. Granularity Selection

The runtime should explicitly decide whether the query asks for:

1. overview/topic root
2. guide/intermediate topic
3. leaf/reference page

This decision should be semantic and contextual, not keyword-driven.

### 4. Topic-Conditioned Page Selection

Once a topic node is selected:

1. if the query is topic-level, return the topic root representative
2. if the query is guide-like, descend one level
3. if the query is reference-like, descend to the leaf branch

## Scientific Support

The proposal is not arbitrary. It is aligned with several research lines.

### A. Hierarchical retrieval outperforms flat retrieval when corpus structure matters

`Hierarchical Semantic Retrieval with Cobweb` argues that flat neural retrieval underuses corpus structure and proposes internal prototype nodes with coarse-to-fine traversal:

- https://arxiv.org/abs/2510.02539

Relevance to Needle-X:

1. internal nodes can carry multi-granular relevance signals
2. prototype trees improve robustness when plain kNN degrades
3. retrieval paths become more interpretable

This is directly aligned with `topic nodes`.

### B. Multi-granular graph reasoning is useful because evidence exists at different levels

`Hierarchical Graph Network for Multi-hop Question Answering` models questions, paragraphs, sentences, and entities in one graph:

- https://arxiv.org/abs/1911.03631

Relevance:

1. different tasks require different node granularities
2. a single flat unit of retrieval is not enough
3. hierarchical node types are operationally useful, not just theoretically elegant

Needle-X has the same failure mode:

- overview queries and leaf queries should not share the same retrieval unit

### C. Hierarchical graph retrieval improves evidence coherence

`SentGraph: Hierarchical Sentence Graph for Multi-hop Retrieval-Augmented Question Answering` explicitly argues that chunk-based flat retrieval often yields incoherent evidence and replaces it with topic-level subgraphs and graph-guided expansion:

- https://arxiv.org/abs/2601.03014

Relevance:

1. flat chunk retrieval can miss logical structure
2. topic-level subgraphs improve retrieval coherence
3. retrieval should respect latent structure, not just local similarity

This supports topic-root inference as a coherence mechanism.

### D. Logical hierarchy extraction helps downstream retrieval

`Extracting Variable-Depth Logical Document Hierarchy from Long Documents` shows that recovering document hierarchy can improve passage retrieval:

- https://arxiv.org/abs/2105.09297

Relevance:

1. document structure is recoverable
2. hierarchy is useful for retrieval, not only parsing
3. variable-depth logical hierarchy is compatible with topic-root nodes

### E. Realistic retrieval benchmarks already show that flat keyword/semantic retrieval is insufficient

`BRIGHT: A Realistic and Challenging Benchmark for Reasoning-Intensive Retrieval` shows that strong dense retrievers perform poorly when retrieval requires more than shallow semantic overlap:

- https://arxiv.org/abs/2407.12883

Relevance:

1. semantic nearest-neighbor alone is not enough
2. more reasoning-sensitive retrieval is needed
3. query understanding and structured retrieval matter

This supports the claim that page-level flat retrieval is not a stable end-state.

### F. Dense retrieval has inherent representational limitations

Recent work argues that flat dense retrieval has theoretical and practical limits:

1. `Does Generative Retrieval Overcome the Limitations of Dense Retrieval?`
   - https://arxiv.org/abs/2509.22116
2. `Generative Retrieval Overcomes Limitations of Dense Retrieval but Struggles with Identifier Ambiguity`
   - https://arxiv.org/abs/2604.05764
3. `Scaling Laws for Embedding Dimension in Information Retrieval`
   - https://arxiv.org/abs/2602.05062

Relevance:

1. single-vector inner-product retrieval has low-rank constraints
2. as tasks become more complex, those constraints become more visible
3. moving to hierarchical or symbolic identifiers is a plausible response

Needle-X does not need full generative retrieval to benefit from this lesson.
Topic nodes are a pragmatic intermediate representation.

### G. The cluster hypothesis still matters, but only in the right form

Classic IR work on the cluster hypothesis remains relevant:

1. `Reexamining the Cluster Hypothesis: Scatter/Gather on Retrieval Results`
   - https://userpages.cs.umbc.edu/nicholas/clustering/hearst96reexamining.pdf
2. Stanford IR book chapter on clustering in IR
   - https://nlp.stanford.edu/IR-book/html/htmledition/clustering-in-information-retrieval-1.html

Relevance:

1. similar documents tend to be relevant together
2. but clustering only helps if used as a retrieval aid, not as a blind post-processing trick

This matters for Needle-X:

- naive clustering was not enough
- topic-root nodes should be treated as retrieval primitives, not cosmetic rerank boosts

## Devil's Advocate

The idea is strong, but there are serious risks.

### 1. Wrong abstraction level

A synthetic topic node may be less useful than a real page if:

1. the query is actually leaf-specific
2. the inferred node is too broad
3. the cluster is semantically mixed

This is a real risk.

### 2. Cluster contamination

Dense documentation trees often contain:

1. reference pages
2. guides
3. tutorials
4. index pages
5. mirrored or translated branches

If these all collapse into one topic node, the synthetic root may become semantically muddy.

### 3. Topic-root election can silently hard-code site structure

If root inference depends too much on path ancestry, it becomes:

1. site-template dependent
2. less portable across domains
3. vulnerable to weird URL structures

So ancestry can help, but it cannot be the only signal.

### 4. Topic nodes can become stale

If the underlying pages evolve:

1. the synthetic summary can drift
2. the node embedding can become obsolete
3. the representative page can stop being canonical

So topic nodes need lifecycle management, not one-shot construction.

### 5. This might still not beat a stronger semantic selector

A critic could argue:

- "the problem is not that we need topic nodes, but that we need a better semantic model"

This objection is valid.
The correct response is empirical:

- if topic-node retrieval does not improve the benchmark, we should not keep it

## Outside-Context Inspirations

These are not direct literature claims, but design inspirations worth testing.

### A. Latent-variable view

Pages are observations.
Topic roots are latent variables.

This reframes retrieval from:

- nearest-page search

to:

- posterior inference over latent topic states

### B. Barycenter view

The relevant answer for an overview query may be:

- the semantic center of a family

not:

- the densest page in that family

### C. Minimum-description view

A good topic root explains many descendants with low complexity.

This suggests a scoring principle:

- maximize semantic coverage
- minimize granularity complexity

### D. Potential-field view

Pages form an attractor field.
The right answer for overview retrieval is not the point mass with the highest local similarity.
It is the stable basin of the semantic family.

## Concrete Implementation Plan

### Phase 1: Persisted Topic Nodes

Add a `topic_nodes` table to discovery memory with:

1. `topic_id`
2. `host`
3. `root_path`
4. `language`
5. `semantic_summary`
6. `embedding_ref`
7. `support_count`
8. `child_count`
9. `representative_url`
10. timestamps

### Phase 2: Topic Node Construction

Build/update nodes during observe or rebuild-index by:

1. grouping descendants by host and ancestor path
2. aggregating semantic summaries
3. keeping only candidates with enough support
4. generating a topic embedding

### Phase 3: Topic-First Search

Memory search becomes:

1. retrieve topic nodes
2. retrieve pages
3. fuse results with explicit granularity control

### Phase 4: Granularity Inference

Use semantic/contextual signals to infer:

1. overview
2. guide
3. reference

without monolingual keyword heuristics as the primary driver.

### Phase 5: Topic-to-Page Realization

If a topic node wins:

1. return its representative page for overview requests
2. descend into child clusters for guide/reference requests

## Evaluation Plan

Primary benchmark:

1. MDN multilingual overview cases

Secondary benchmarks:

1. CONI official-site cases
2. SQLite/Python overview vs deep page cases
3. warm-state discovery-memory benchmark

Metrics:

1. selected URL pass rate
2. public provider activation rate
3. local provider activation rate
4. topic-root vs leaf confusion rate
5. latency delta

Success criteria:

1. improve MDN overview warm-state pass rate without regressing guide cases
2. keep local-first provider rate high
3. avoid new reliance on monolingual lexical heuristics

## Decision Rule

This proposal should be kept only if it moves the benchmark.

If persisted topic nodes and topic-first retrieval do not improve:

1. MDN overview warm-state pass rate
2. local-first correctness

then the architecture is not justified and should be rolled back.

## References

1. Hearst, M. A., Pedersen, J. O. Reexamining the Cluster Hypothesis: Scatter/Gather on Retrieval Results. SIGIR 1996.
   - https://userpages.cs.umbc.edu/nicholas/clustering/hearst96reexamining.pdf
2. Fang et al. Hierarchical Graph Network for Multi-hop Question Answering. 2019.
   - https://arxiv.org/abs/1911.03631
3. Tang et al. Extracting Variable-Depth Logical Document Hierarchy from Long Documents. 2021.
   - https://arxiv.org/abs/2105.09297
4. Su et al. BRIGHT: A Realistic and Challenging Benchmark for Reasoning-Intensive Retrieval. 2024.
   - https://arxiv.org/abs/2407.12883
5. Zhang et al. Does Generative Retrieval Overcome the Limitations of Dense Retrieval? 2025.
   - https://arxiv.org/abs/2509.22116
6. Gupta et al. Hierarchical Semantic Retrieval with Cobweb. 2025.
   - https://arxiv.org/abs/2510.02539
7. Liang et al. SentGraph: Hierarchical Sentence Graph for Multi-hop Retrieval-Augmented Question Answering. 2026.
   - https://arxiv.org/abs/2601.03014
8. Bracher, Vakulenko. Generative Retrieval Overcomes Limitations of Dense Retrieval but Struggles with Identifier Ambiguity. 2026.
   - https://arxiv.org/abs/2604.05764
9. Scaling Laws for Embedding Dimension in Information Retrieval. 2026.
   - https://arxiv.org/abs/2602.05062
