# Consensus
Mandatory 4 distributed systems course

The following is a tutorial on how to use the program:
- This implementation of the Ricart-Argawala algorithm is hardcoded to use 4 nodes so the program is not without limitations
- To run the program open 4 terminal windows
- From the system path ../Consensus/Nodes in all 4 windows run seperate commands:
    - 1st window: go run node.go Node1 localhost:5001
    - 2nd window: go run node.go Node2 localhost:5002
    - 3rd window: go run node.go Node3 localhost:5003
    - 4th window: go run node.go Node4 localhost:5004
- This will execute the 4 nodes that have been hardcoded into the nodes map in the program and they will start sending requests and replies to each other
