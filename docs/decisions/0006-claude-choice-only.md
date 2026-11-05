# 0006 Claude picks among local candidates, never edits coordinates

Status: accepted 2026-11-05 (PLAN.md section 8.4).

**Decision.** The Claude tier sends one clean panel per locally generated candidate plus
two clean sample pages and asks for `{choice, split, reasons}` with structured output.
No coordinate adjustment.

**Why.** Live runs on 8 clusters: with free-form adjustment Opus 5.5 scored mean IoU 0.851
with 2 clipped clusters and Haiku 5.5 drifted by up to 30% of the page; choice-only with
panels gave Opus 5.5 0.967 with none clipped, Sonnet 5.5 and Haiku 5.5 0.961 with one
borderline flag each. Default model `claude-opus-5-5`, Sonnet 5.5 as the budget option.

**Costs.** 5 to 7 images per cluster, about 3 c on Opus 5.5 and 1.5 c on Sonnet 5.5;
`max_tokens` 8192 because thinking tokens count against it.
