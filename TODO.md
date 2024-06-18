# TODOs

- Improve checkpoints/archives
  - Store information in each buffer if already archived
  - Do not create new checkpoint if all buffers already archived
- Missing Testcases:
  - General tests
  - Check for corner cases that should fail gracefully
  - Write a more realistic `ToArchive`/`FromArchive` tests
- Optimization: Once a buffer is full, calculate min, max and avg
  - Calculate averages buffer-wise, average weighted by length of buffer
  - Only the head-buffer needs to be fully traversed
- Optimization: If aggregating over hwthreads/cores/sockets cache those results
  and reuse some of that for new queres aggregating only over the newer data
- ...
