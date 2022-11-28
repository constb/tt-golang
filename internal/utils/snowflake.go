package utils

import (
	"math/rand"

	"github.com/bwmarrin/snowflake"
)

const snowflakeEpoch = 1669205840566

var nodeId = rand.Int63n(1024)
var node *snowflake.Node

func init() {
	snowflake.Epoch = snowflakeEpoch
	var err error
	node, err = snowflake.NewNode(nodeId)
	if err != nil {
		panic(err)
	}
}

func GenerateID() snowflake.ID {
	return node.Generate()
}
