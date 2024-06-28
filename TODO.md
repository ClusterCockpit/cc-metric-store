# Possible Tasks and Improvements

Importance:

- **I** Important
- **N** Nice to have
- **W** Won't do. Probably not necessary.

- Benchmarking
  - Benchmark and compare common timeseries DBs with our data and our queries (N)
- Memory management
  - To overcome garbage collection overhead: Reimplement in Rust (N)
  - Request memory directly batchwise via mmap (started in branch) (W)
- Archive
  - S3 backend for archive (I)
  - Store information in each buffer if already archived (N)
  - Do not create new checkpoint if all buffers already archived (N)
- Checkpoints
  - S3 backend for checkpoints (I)
  - Combine checkpoints into larger files (I)
  - Binary checkpoints (started in branch) (W)
- API
  - Redesign query interface (N)
  - Introduce JWT authentication for REST and NATS (I)
- Testing
  - General tests (I)
  - Test data generator for regression tests (I)
  - Check for corner cases that should fail gracefully (N)
  - Write a more realistic `ToArchive`/`FromArchive` Tests (N)
- Aggregation
  - Calculate averages buffer-wise as soon as full, average weighted by length of buffer (N)
  - Only the head-buffer needs to be fully traversed (N)
  - If aggregating over hwthreads/cores/sockets cache those results and reuse
    some of that for new queries aggregating only over the newer data (W)
- Compression
  - Enable compression for http API requests (N)
  - Enable compression for checkpoints/archive (I)
- Sampling
  - Support data re sampling to reduce data points (I)
  - Use re sampling algorithms that preserve min/max as far as possible (I)
