# Troubleshooting Guide

This guide provides solutions to common problems you might encounter while deploying or operating CallFS.

## Service Startup Failures

If the CallFS service fails to start, check the following:

#### 1. Configuration Errors
- **Symptom**: The service exits immediately with a configuration-related error message in the logs.
- **Solution**:
    - Run the validation command: `./callfs config validate --config /path/to/config.yaml`.
    - Carefully check your `config.yaml` for syntax errors, incorrect data types, or missing required fields (like `api_keys` or `dsn`).

#### 2. Port Conflicts
- **Symptom**: Logs show an "address already in use" error.
- **Solution**:
    - Ensure that the port specified in `server.listen_addr` (default `:8443`) is not being used by another application.
    - Use `sudo lsof -i :8443` or `sudo netstat -tulpn | grep 8443` to find the conflicting process.

#### 3. TLS Certificate Issues
- **Symptom**: Errors related to "certificate not found" or "invalid certificate".
- **Solution**:
    - Verify that the paths in `server.cert_file` and `server.key_file` are correct.
    - Ensure the user running CallFS has read permissions for the certificate and key files.
    - For development, you can quickly generate self-signed certificates (see the Installation guide).

#### 4. Database or Redis Connection Issues
- **Symptom**: The service fails to start with errors about connecting to PostgreSQL or Redis.
- **Solution**:
    - Verify that your PostgreSQL and Redis servers are running and accessible from the CallFS node.
    - Check that the `metadata_store.dsn` and `dlm.redis_addr` in your configuration are correct.
    - Test the connection manually from the Callfs server:
      ```bash
      # Test PostgreSQL
      psql "postgres://user:pass@host:port/db" -c "SELECT 1;"
      # Test Redis
      redis-cli -h <redis-host> -p <port> ping
      ```

## API Request Failures

#### HTTP 401 Unauthorized
- **Cause**: The API key is missing, incorrect, or not properly formatted.
- **Solution**:
    - Ensure you are providing the API key in the `Authorization` header with the `Bearer` prefix: `Authorization: Bearer <your-api-key>`.
    - Verify that the key you are using is listed in the `auth.api_keys` section of your configuration.

#### HTTP 403 Forbidden
- **Cause**: The authenticated user does not have the required Unix permissions to perform the operation on the file or directory.
- **Solution**:
    - Check the permissions of the target file/directory in the metadata store.
    - This is an intentional security feature. Adjust permissions if the access should be allowed.

#### HTTP 404 Not Found
- **Cause**: The requested file or directory path does not exist.
- **Solution**:
    - Double-check the path for typos.
    - Remember that directory paths often require a trailing slash (`/`).

#### HTTP 409 Conflict
- **Cause**: You are trying to `POST` (create) a file or directory that already exists somewhere in the cluster.
- **Solution**:
    - This is a protective measure. If you intend to update the file, use the `PUT` method instead.
    - The error response body will contain details about where the conflicting file exists.

## Performance Issues

#### Slow Response Times
- **High Latency on File Operations**:
    - **Check Backend Performance**: If using S3, check the latency between your CallFS node and the S3 endpoint. If using LocalFS, check the disk I/O performance (`iostat -x`).
    - **Check Database Performance**: Slow metadata operations can delay file access. Monitor your PostgreSQL instance for slow queries.
- **High CPU or Memory Usage**:
    - **Enable Profiling**: Go's `pprof` endpoints can be enabled for detailed performance analysis.
    - **Check for Excessive Logging**: In `debug` mode, logging can be resource-intensive. Ensure you are using `info` level in production.
    - **Monitor Metrics**: Use the `/metrics` endpoint to identify bottlenecks. High `callfs_backend_op_duration_seconds` or `callfs_metadata_db_query_duration_seconds` can point to the source of the slowdown.

## Distributed Locking Issues

- **Symptom**: Operations hang or time out with "failed to acquire lock" errors.
- **Cause**: This can happen if a process holding a lock crashes without releasing it, leaving behind an orphaned lock in Redis.
- **Solution**:
    - **Manual Intervention (Emergency)**: You can manually delete the stale lock key from Redis. The keys are prefixed with `callfs:lock:`.
      ```bash
      # Find the lock key for a specific path
      redis-cli keys "callfs:lock:/path/to/locked/file"
      # Delete the key
      redis-cli del <the-lock-key>
      ```
    - **Check for Network Issues**: Ensure all CallFS nodes have stable, low-latency connections to the Redis cluster.
