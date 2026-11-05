package main

import (
	"fmt"
	"path/filepath"
	"pulsar/internal/archive"
	"slices"
	"strings"
)

type TreeNode struct {
	Name     string
	IsDir    bool
	Children map[string]*TreeNode
}

func buildTree(entries []archive.FileEntry) *TreeNode {
	root := &TreeNode{
		Name:     "",
		IsDir:    true,
		Children: make(map[string]*TreeNode),
	}

	for _, entry := range entries {
		parts := strings.Split(filepath.ToSlash(entry.Filename), "/")

		current := root
		for i, part := range parts {
			if part == "" {
				continue
			}

			child, exists := current.Children[part]

			if !exists {
				child = &TreeNode{
					Name:     part,
					IsDir:    i < len(parts)-1 || entry.Type == archive.Directory,
					Children: make(map[string]*TreeNode),
				}

				current.Children[part] = child
			}

			current = child
		}
	}

	return root
}

func printTree(node *TreeNode, prefix string) {

	keys := make([]string, 0, len(node.Children))

	for key := range node.Children {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	for i, key := range keys {
		child := node.Children[key]

		isLast := i == len(keys)-1

		connector := "├── "
		nextPrefix := prefix + "│   "

		if isLast {
			connector = "└── "
			nextPrefix = prefix + "    "
		}

		name := child.Name
		if child.IsDir {
			name += "/"
		}

		fmt.Println(prefix + connector + name)

		if child.IsDir {
			printTree(child, nextPrefix)
		}
	}
}

func list(path string) error {

	reader := archive.ArchiveReader{}
	if err := reader.OpenArchive(path); err != nil {
		return err
	}

	defer reader.Close()

	// fmt.Println("Archive contents:")

	//build the tree
	tree := buildTree(reader.Files)
	printTree(tree, "")

	return nil
}
