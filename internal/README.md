# Internal Documentation

## Things I would like to improve...

While developing ClusterCockpit, there are a lot of things I learned and that I would change if redoing everything. Unfortunately, I did not have as much time as I hopped and a few things ended up a bit chaotic. This is partly because the requirements and features and metrics (and particularly their scopes) changed, PHP Symfony ate up a lot of dev. time, and it was my first big frontend. The frontend ended up particularly chaotic, but it works and I really like Svelte.

The `cc-metric-store`, in my opinion, however, is one of the more interesting aspects of the ClusterCockpit stack, with very very interesting and effective possibilities for improvement! It was created just in a span of two months and is successfully deployed for multiple clusters. Yes, trying to implement your own time-series data-base might be crazy and naive, but where would you do something like that if not in university? I personally would not even call the metric store a TSDB for now because it lacks a lot of features. However, in our defense: there are not that many open source in-memory TSDBs. The closest thing I found was a [redis extension](https://github.com/RedisTimeSeries/RedisTimeSeries), but it was lacking horizontal aggregation and other features. The core philosophy should not change and basically is __KISS__ (*keep it simple stupid*):

- Because of the job-archive, only the last 48h or so need to be stored in a TSDB. This means that even for large clusters with lots of cores, *storing everything in main-memory* is very much feasible.
- In case of a power outage etc, missing monitoring data is the least of the admins problems: *Redundancy and Consistency is not important*.
- The metrics are always written and queried along a simple hierarchy: `cluster -> host -> ...`. This means that *only a single tree-like index is needed* (aggregations along this hierarchy are common and need to be fast!).
- All measurements are done in fixed intervals. There is *no need to know exactly when something was measured*, only within what minute/"slot" a measurement was taken (a slot can cover 10s or so).

Here is a loose list of things I would do better if I could do it again. Please feel free to contact [lou.knauer@fau.de](mailto:lou.knauer@fau.de) before attempting to extend and improve this implementation. Because it is not much code and there still are lots of opportunities for improvement, in my opinion, it would make a good thesis of master project. If it is done well and the interface becomes truly generic and application independent, it could even become useful for others as well because, as I mentioned before, open-source in-memory TSDBs are not that common.

- Checkpoints/Archiving:
  - Time-series data can be compressed using very delta-encoding or XORs with trailing/leading zeros: [The gorilla paper has some examples](https://www.vldb.org/pvldb/vol8/p1816-teller.pdf).
  - A few large files are better than a lot of small ones (*ongoing work*).
  - Use a binary file format instead of JSON for faster en-/decoding (*ongoing work*).
  - Make archiving more flexible, allow to move archives into another DB (for long-term storage).
- API:
  - Writing:
    - Currently, the line-protocol decoder is very strongly coupled to the format used by ClusterCockpit. Make it more flexible and generic. *A single strict hierarchy is still a must!*.
    - Blank TCP/TLS write endpoints: Keep-alive connections without the protocol-overhead of HTTP(S). JWT (and optionally default tags) must only be transfered once.
  - Reading:
    - Re-do the query API completely! Allow for much more komplex queries with proper query language, such as for example: `select(from: 2022-01-08T00:00:00, to: 2022-01-09T23:59:59) > filter(hostname: ["e0001", "e0002", ...]) > topology-aggregation("sum", ["core0", "core1", "core2", "core3"]) > transform(flops_any: flops_dp * 2 + flops_sp) > metrics(flops_any)`
- Internals:
  - Not urgent because Go actually does quite a good job: rewrite in C/C++ or Rust (or another language without garbage collection)
  - Alternatively: Implement a custom allocator for the chunks that bypasses GC and uses `mmap`.
  - Combine chunk-head (start-time, frequency, is-checkpointed, ...) and payload slice in the same allocation?
  - Calculate stats once after a chunk is closed.
  - Implement all the things needed for efficient and more complex queries.
- ...

