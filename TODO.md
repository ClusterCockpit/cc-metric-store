# TODO

- Delete this file and create more GitHub issues instead?
- Missing Testcases:
    - Port at least all blackbox tests from the "old" `MemoryStore` to the new implementation
    - Check for corner cases that should fail gracefully
    - Write a more realistic `ToArchive`/`FromArchive` tests
    - Test edgecases for horizontal aggregations
- Optimization: Once a buffer is full, calculate min, max and avg
    - Calculate averages buffer-wise, average weighted by length of buffer
    - Only the head-buffer needs to be fully traversed
- ...
