# ADR-0047 Probe Forensic Analysis — Why the Gate Rejected

> Per-query, evidence-based analysis of the failed embedding representation probe.
> Method: a python replica of the eval runner (validated: v3/A avg NDCG@5 = 0.7376 vs runner 0.738, limit-5 HNSW match) against the four GPU collections (`arca_probe_a/b/c/d`, 3017 chunks each).
> No code or retrieval changes were made. All claims cite reproduced scores/IDs.

---

## 1. v3 regressions (all queries, sorted by A→B delta)

| query | intent | A | B | C | D | dA→B |
|---|---|---|---|---|---|---|
| g-sf-07 | concept | 1.000 | 0.000 | 0.000 | 0.000 | **−1.000** |
| g-sf-14 | concept | 1.000 | 0.000 | 0.237 | 0.000 | **−1.000** |
| g-sf-10 | concept | 1.000 | 0.237 | 0.613 | 0.613 | −0.763 |
| g-cmp-04 | comparison | 0.877 | 0.387 | 0.387 | 0.307 | −0.490 |
| g-sf-13 | concept | 0.850 | 0.613 | 0.920 | 0.850 | −0.237 |
| g-sf-06 | single_fact | 0.920 | 0.693 | 0.307 | 0.387 | −0.226 |
| g-sf-12 | single_fact | 1.000 | 0.877 | 0.877 | 0.920 | −0.123 |
| g-sf-04 | concept | 1.000 | 0.920 | 0.920 | 0.613 | −0.080 |
| g-sf-08 | concept | 1.000 | 0.920 | 0.877 | 0.613 | −0.080 |
| g-ent-08 | entity | 0.637 | 0.559 | 0.246 | 0.195 | −0.078 |
| g-cmp-01 | comparison | 0.624 | 0.613 | 0.613 | 0.000 | −0.011 |
| g-ent-04 | entity | 0.339 | 0.339 | 0.553 | 0.339 | ±0.000 |
| g-ent-05 | entity | 0.170 | 0.214 | 0.000 | 0.000 | +0.044 |
| g-sf-05 | concept | 0.850 | 0.920 | 1.000 | 1.000 | +0.069 |
| g-cmp-02 | comparison | 0.500 | 0.631 | 1.000 | 0.000 | +0.131 |
| g-sf-11 | concept | 0.624 | 0.920 | 0.920 | 0.693 | +0.296 |
| g-sf-03 | concept | 0.693 | 1.000 | 1.000 | 1.000 | +0.307 |
| g-ent-01 | entity | 0.307 | 0.920 | 0.651 | 0.651 | +0.613 |

(Remaining v3 queries: unchanged at 1.000/1.000 or 0.000 across reps.)

## 2. Top-10 comparison — the mechanism, with scores

### g-sf-07 "What role do repositories play in DDD applications?" (A=1.0 → B=0.0)

Expected: `...implementing-the-repository-pattern-in-golang/001, 002`.

| rep | top-5 (score, section) |
|---|---|
| A | 001 0.756 [Implementing the repos…] **✓** · 002 0.752 **✓** · 001 0.748 [Exploring Factories, R…] · 002 0.746 · 012 0.736 [Implementing our produ…] |
| B | 012 0.756 [**Implementing our produ…**] · 001 0.744 [Applying DDD to an exi…] · 002 0.734 [Exploring Factories…] · … **0 hits** |
| C | 012 0.762 [Implementing our produ…] · 001 0.749 [Applying DDD…] · **0 hits** |
| D | 001 0.731 [Applying DDD…] · **0 hits** |

The chunk that jumped from A-rank-5 to B-rank-1 is `012` of **a different section** whose heading contains "repository" ("Implementing our production-ready…"). The heading token "repository" matches the query token "repositories" and overrides the content signal.

### g-sf-14 "How does transactional messaging work…" (A=1.0 → B=0.0)

Expected: `...transactional-messaging-updating-the-depot…`, `...transactional-messaging-identifying-problems…`.

| rep | top-5 |
|---|---|
| A | 004 0.785 [Updating the Depot mod…] **✓** · 004 0.757 [Identifying problems i…] **✓** · … |
| B | 002 0.779 [**Transactional Messagin…**] · 001 0.772 [Transactional Messagin…] · 003 0.756 [Transactional Messagin…] · **0 hits** |
| C | 002 0.766 [Updating the handlers…] · … 004 0.745 [Identifying problems…] (1 hit) |
| D | 001 0.796 [subscriber := containe…] · **0 hits** |

"Transactional Messaging" exists as its **own section** (several chunks); the expected chunks live under a different section ("Consequences What becomes…"). In B, the heading-match section displaces the content-match section.

### h-es-01 "What does the Preface of Event-Driven Architecture in Golang say?" (0 across all reps)

Threshold-less scores are high (0.83–0.88) but the expected chunk (`table-of-contents-preface/001`) never enters top-8 in any rep. "Preface" appears across **21 chunks** in the Stack book (preface + who-this-book-is-for + what-this-book-covers …). The specific expected chunk's content ("Companies are adopting event-driven architecture…") matches the query poorly; other preface-ish chunks win.

## 3. Embedding drift — real inputs and measured shift

Real inputs for `rick-rubin/tuning-in/002` (payload-verified):

```
A: Think of the universe as an eternal creative unfolding. Trees blossom…
B: Tuning In\n\nThink of the universe…
C: Tuning In\n\nThink of the universe…          ← B ≡ C (single-segment path)
D: Robert Henri > Tuning In\n\nThink of the universe…   ← title = TitleResolver failure ("Robert Henri")
```

Real inputs for the Stack preface chunk:

```
B: Preface\n\nCompanies are adopting **event-driven architecture** (**EDA**)…
C: Table of Contents > Preface\n\n…
D: Event-Driven Architecture in Golang > Table of Contents > Preface\n\n…
```

Measured vector shift (same chunk, different reps; all norms = 1.000, L2):

| chunk | cos(A,B) | cos(A,C) | cos(A,D) |
|---|---|---|---|
| rick-rubin/tuning-in/002 | — (B=C identical input) | — | 0.965 |
| Stack preface/001 | 0.9966 | 0.9945 | 0.9645 |
| Boyle distributed-system/001 | 0.9861 | 0.9468 | 0.9237 |

**Corpus-wide (n=200 sample): avg cos(A,B) = 0.9512 · avg cos(A,D) = 0.8845.** A 5% vector change is enough to reorder rankings (g-sf-07's decisive jump was 0.736 → 0.756); D's 11.5% shift is a much larger perturbation.

## 4. Shared cause of the regressions — measured corpus statistics

| metric | value | evidence |
|---|---|---|
| sections with >1 chunk | **471/1120 (42%)** | scroll of all 3017 payloads |
| 3-segment section paths | **2210/3017 (73%)** | segment distribution: {3: 2210, 2: 574, 1: 233} |
| generic last segments | 156 chunks (5.2%) | "Preface/Summary/…" |
| worst duplicates | 38 × "…CoffeeCo sy…", 27 × "Consequences What becomes…", 21 × preface-family | scroll |
| query-token-in-heading collisions | g-sf-07 ("repository" in ≥2 sections), g-sf-14 ("Transactional Messaging" own section), h-es-01 (21 preface chunks) | top-10 payloads |

The common mechanism: **a query term that occurs in a heading boosts every chunk of that section regardless of content.** Because heading repetition is structurally common in this corpus (42% multi-chunk sections, 73% deep paths, book TOCs like the 38-chunk CoffeeCo section), the boost fires often and displaces content-matched chunks.

## 5. B vs C — not identical

Per-query comparison of retrieved top-5 lists: **differs on 41/51 queries, identical on 10**. The aggregate metrics matched (0.377/0.377) because gains and losses cancelled across queries, not because the embeddings are equal. Identical cases are mostly single-segment paths (rick-rubin: B ≡ C input by construction). Examples of genuine diffs (v3): g-sf-15, g-cmp-01/02, g-ab-02/03/04/06/07/08; (v4): h-bk-01/02, h-me-01/02, h-bd-01/02/03, h-es-01/02/03.

## 6. Why D is worst

- **Largest drift**: avg cos(A,D) = 0.8845 vs 0.9512 (A→B) — the book-title prefix is a bigger perturbation than the section prefix.
- **Title quality failure**: rick-rubin's resolved title is "Robert Henri" (TitleResolver chain fallback, documented in the handbook). D's only v4 win (h-rr-02, 0.613) is attributable to this *wrong* title: the query contains "artist", and "Robert Henri" is an artist's name — a lucky collision, not a design success.
- **Cross-book disambiguation did not help**: h-bd-02 ("Preface of Domain-Driven Design with Golang") drops from A=0.500 to D=0.000. The book title is a *generic* query-term contributor; every chunk of the target book shares it, so it does not discriminate within the book, while 5 books' titles add cross-book noise to every query.
- **Per-slice**: D is worst or tied-worst in every v3 slice (comparison 0.250, concept 0.692, single_fact 0.861) and only marginally different from B/C on the heading slice (0.367 vs 0.377).

## 7. Query taxonomy — slice metrics (avg NDCG@5)

| slice | n | A | B | C | D |
|---|---|---|---|---|---|
| heading (v4) | 13 | 0.225 | **0.377** | **0.377** | 0.367 |
| entity (v3) | 8 | 0.306 | 0.379 | 0.306 | 0.273 |
| comparison (v3) | 4 | 0.750 | 0.658 | 0.750 | 0.250 |
| single_fact (v3) | 5 | 0.984 | 0.914 | 0.837 | 0.861 |
| concept (v3) | 12 | 0.918 | 0.711 | 0.780 | 0.692 |

The heading gain is real but narrow: it comes from **3 queries** (h-rr-01, h-rr-03, h-bd-03 — verbatim section titles). Six heading queries score 0.000 in **all** representations (h-bk-02, h-me-01, h-me-02, h-es-01, h-es-02, h-es-03) — partly gold-set quality (weak expected-chunk selection, e.g. one-stock-systems/001-002 while the section has 16 chunks and the best content match is chunk 009; paraphrase queries with no lexical anchor).

## 8. Root cause (single sentence, evidence-backed)

**The heading prefix boosts every chunk whose section heading shares a query term, overriding content relevance — and this corpus is structurally full of repeated headings (42% of sections have >1 chunk, 73% of paths are 3 segments, book TOCs repeat headings up to 38×), so the boost fires often and displaces content-matched chunks (g-sf-07: "repository" in another section jumps 5th→1st; g-sf-14: the "Transactional Messaging" section displaces the "Consequences…" section that actually answers).**

D is not a fix for the same reason plus more: the book-title prefix is a larger, non-discriminating perturbation (avg drift 11.5% vs 4.9%) on top of an unreliable title signal ("Robert Henri").

---

## 9. What was answered / what remains open

### Answered

- Which v3 queries regress and by how much (table §1, per-rep scores).
- The exact displacement mechanism with chunk IDs, sections, and scores (§2): heading-token matching overrides content matching.
- B and C are not the same: 41/51 queries differ; identical aggregates are cancellation, not equality (§5).
- Why D is worst: largest drift + broken title signal + non-discriminating book prefix (§6).
- Heading slice gain is real but narrow: 3 verbatim-title queries drive +15 pp; 6 queries fail in every representation (§7).
- Embedding drift magnitudes: avg cos(A,B)=0.9512, avg cos(A,D)=0.8845, norms stable at 1.0 (§3).

### Open

- Whether a *weighted* heading inclusion (heading appended with lower relative weight, or only the last segment) would keep the 3 query gains without the §2 displacement — **not tested, no evidence**.
- Whether goldset v4's 6 dead queries reflect retrieval failure or curation failure (expected-chunk selection, e.g. one-stock-systems 001-002 vs 009) — needs a gold-set review pass, not a retrieval change.
- Whether the displacement is specific to nomic-embed-text or generalizes to other embedding models — **not tested, no evidence**.
- Why exactly the Stack preface chunk (h-es-01) never surfaces despite high neighbouring scores — the 21-way "Preface" collision is documented, but the per-token contribution is not isolated.
