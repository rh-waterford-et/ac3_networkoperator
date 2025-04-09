#!/bin/bash

# Set the Skupper service address (ClusterIP:Port)
export SKUPPER_SERVICE="172.30.7.218:8080"  # Adjusted for your environment

# Print the service address to verify
echo "Testing Skupper service at: http://$SKUPPER_SERVICE"

# Loop to test the connection
for i in {1..5}; do
  echo "Attempt $i:"
  
  # Perform the cURL test
  curl -v http://$SKUPPER_SERVICE

  # Pause for 1 second between tests
  sleep 1
done
