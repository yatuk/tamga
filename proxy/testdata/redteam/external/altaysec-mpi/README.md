---
license: cc-by-4.0
language: [en, tr]
task_categories: [text-classification]
tags: [prompt-injection, jailbreak, llm-security, ai-security, red-teaming, turkish, owasp-llm-top10, security]
pretty_name: "Multilingual Prompt Injection (EN + TR)"
size_categories: [n<1K]
configs: [{config_name: default, data_files: prompt_injection.jsonl}]
---

![banner](banner.png)

# Multilingual Prompt Injection

```python
from datasets import load_dataset
ds = load_dataset("fevziegeyurtsevenler/multilingual-prompt-injection")
```

**217** labeled prompt-injection & jailbreak techniques across **8 categories**, ~half **Turkish** — because non-English attacks slip past English-only filters. Every row ships with its **defense**. Defensive dataset (canary placeholders `{SECRET}`/`{URL}`; no real harmful content).

Columns: `category, language, technique, payload, target_behavior, owasp, atlas, defense, severity`. Categories: direct/indirect injection, jailbreak-persona, system-prompt-leak, data-exfiltration, encoding, Turkish morphological, tool/agent abuse.

## Schema
| column | meaning |
|---|---|
| technique | technique name |
| payload | example ({SECRET}/{URL} placeholders) |
| target_behavior | expected bad outcome |
| owasp, atlas, severity | mapping |
| defense | mitigation |

## Related AltaySec resources
- 🕵️ **uncloak** — hidden prompt-injection scanner: [GitHub](https://github.com/fevziegeyurtsevenler/uncloak) · [live demo](https://fevziegeyurtsevenler.github.io/uncloak/)
- 📚 Datasets: [detection-rules](https://huggingface.co/datasets/fevziegeyurtsevenler/prompt-injection-detection-rules) · [glossary](https://huggingface.co/datasets/fevziegeyurtsevenler/ai-security-glossary) · [skills-in-the-wild](https://huggingface.co/datasets/fevziegeyurtsevenler/skills-in-the-wild) · [agent-skill-vulnerabilities](https://huggingface.co/datasets/fevziegeyurtsevenler/agent-skill-vulnerabilities)
- 🌐 [AltaySec](https://altaysec.com.tr) · [Açık Kaynak Lab](https://altaysec.com.tr/acik-kaynak)

## Citation
```bibtex
@misc{yurtsevenler2026multilingualpromptinjection,
  title  = {Multilingual Prompt Injection Dataset (EN+TR)},
  author = {Yurtsevenler, Fevzi Ege},
  year   = {2026},
  publisher = {AltaySec / Hugging Face},
  howpublished = {\url{https://huggingface.co/datasets/fevziegeyurtsevenler/multilingual-prompt-injection}}
}
```

⚠️ Defensive / authorized-use only. Multilingual-first LLM/agent security research by **[AltaySec](https://altaysec.com.tr)**.
