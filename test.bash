#!/bin/bash

# Step 6: Expose the TCP server in the Private site
echo "Exposing TCP server in Private site..."
skupper expose deployment/tcp-server --port 9090

# Check if the exposure was successful
echo "Verifying exposed services..."
skupper service status tcp-server

# Step 7: Run the TCP client in the Public site
echo "Running TCP client in Public site..."
echo "hello" | kubectl run tcp-client --stdin --rm --image=quay.io/ryjenkin/ac3no3:177 --restart=Never -- tcp-server 9090


