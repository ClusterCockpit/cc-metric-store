# TODO

- Delete this file and create more GitHub issues instead?
- Missing Testcases:
    - Port at least all blackbox tests from the "old" `MemoryStore` to the new implementation
    - Check for corner cases that should fail gracefully
    - Write a more realistic `ToArchive`/`FromArchive` tests
    - Test edgecases for horizontal aggregations
- Release Data
    - Implement API endpoint for releasing old data
    - Make sure data is written to disk before it is released
    - Automatically free up old buffers periodically?
- Optimization: Once a buffer is full, calculate min, max and avg
    - Calculate averages buffer-wise, average weighted by length of buffer
    - Only the head-buffer needs to be fully traversed
- Implement basic support for query of most recent value for every metric on every host
- All metrics are known in advance, including the level: Use this to replace `level.metrics` hashmap by slice?
- ...
