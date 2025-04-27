package constants

import "time"

const HttpTimeout = 20 * time.Second
const MaxConcurrentRequests = 15
const RequestsPerSecond = 2.0
const MaxRetries = 3
const InitalBackoff = time.Second
const BackoffFactor = 2.0
