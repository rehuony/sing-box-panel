-- No query reads traffic samples by their intervals: a node's history reads
-- the samples after the start of its range through traffic_samples_history.
-- The interval index only cost a write for every sample.
DROP INDEX traffic_samples_interval_history;
