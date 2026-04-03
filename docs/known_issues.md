# Known Issues


### KI: Vector mode score mismatch
Pure vector search (`--mode vector`) returns scores inconsistent with
direct cosine similarity measured in psql. Likely a parameter binding
issue in searchVector. Hybrid mode works correctly and is suitable
for zero-shot clustering. Investigate before fssorter.