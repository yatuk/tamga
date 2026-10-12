---
license: apache-2.0
language: [tr]
task_categories: [text-classification]
tags: [prompt-injection, llm-security, turkish, guardrails, evaluation, ai-security, red-teaming]
pretty_name: "Guard Blindspots TR"
size_categories: [n<1K]
configs: [{config_name: default, data_files: blindspots_items.jsonl}]
---

![banner](banner.png)

# guard-blindspots-tr

```python
from datasets import load_dataset
ds = load_dataset("fevziegeyurtsevenler/guard-blindspots-tr")
```

248 Turkish prompt injections scored through popular open guard models. **An English-good guard is not automatically Turkish-good.**

| Guard | Turkish miss-rate |
|-------|:-----------------:|
| fmops/distilbert-prompt-injection | 0.0% |
| AltaySec detector | 0.8% |
| protectai/deberta-v3-base-prompt-injection-v2 | 1.6% |
| jackhhao/jailbreak-classifier | **85.5%** |

Each row: `text`, `technique`, `category`, `variant` (original/casefold/zerowidth-spaced/leet), `caught_by_<guard>` (1/0), `n_guards_missed`, `missed_by_all`. The rows a guard missed are a hard-negative set for improving it.

Full method + the over-refusal catch: **[github.com/fevziegeyurtsevenler/guard-blindspots-tr](https://github.com/fevziegeyurtsevenler/guard-blindspots-tr)**

Related: [turkish-casefold-evasion](https://huggingface.co/datasets/fevziegeyurtsevenler/turkish-casefold-evasion) · [guardrail-arena](https://huggingface.co/datasets/fevziegeyurtsevenler/guardrail-arena) · [AltaySec](https://altaysec.com.tr)

Apache-2.0 · by AltaySec.
