package gocity

import (
	"sort"
	"strings"
)

type NodeType string

const (
	StructType  NodeType = "STRUCT"
	FileType    NodeType = "FILE"
	PackageType NodeType = "PACKAGE"
)

type Node struct {
	Name     string
	FullPath string
	Type     NodeType

	Width float64
	Depth float64

	ChildWidth    float64
	ChildDepth    float64
	ChildPosition Position

	NumberOfLines      int
	NumberOfMethods    int
	NumberOfAttributes int

	Children    []*Node
	Line        int
	childrenMap map[string]*Node
}

// Lógica de ordenamiento (Antes en sort.go)
type byChildWidth []*Node

func (s byChildWidth) Len() int      { return len(s) }
func (s byChildWidth) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s byChildWidth) Less(i, j int) bool {
	if s[i].ChildWidth < s[j].ChildWidth {
		return true
	}
	if s[i].ChildWidth > s[j].ChildWidth {
		return false
	}
	return strings.Compare(s[i].Name, s[j].Name) == -1
}

func (n *Node) GenerateChildList() {
	for _, child := range n.childrenMap {
		n.Children = append(n.Children, child)
		if len(child.childrenMap) > 0 {
			child.GenerateChildList()
		}
	}
}

func (n *Node) GenerateFlatChildrenPosition() {
	if n.Type == StructType || len(n.Children) == 0 {
		size := float64(n.NumberOfAttributes) + 1
		n.Width, n.Depth, n.ChildWidth, n.ChildDepth = size, size, size, size
		return
	}

	for _, child := range n.Children {
		child.GenerateFlatChildrenPosition()

		if child.Type == PackageType {
			child.ChildWidth, child.ChildDepth = 5.0, 5.0
		}
	}

	sort.Sort(sort.Reverse(byChildWidth(n.Children)))

	positionGenerator := NewGenerator(len(n.Children))
	for _, child := range n.Children {
		child.ChildPosition = positionGenerator.NextPosition(child.ChildWidth, child.ChildDepth)
	}

	bounds := positionGenerator.GetBounds()
	n.Width, n.Depth = bounds.X, bounds.Y

	for _, child := range n.Children {
		child.ChildPosition.X -= n.Width / 2.0
		child.ChildPosition.Y -= n.Depth / 2.0
	}

	if n.Type == FileType {
		n.Width += float64(n.NumberOfAttributes)
		n.Depth += float64(n.NumberOfAttributes)
	}

	n.ChildWidth, n.ChildDepth = n.Width, n.Depth
}

func getPathAndFile(fullPath string) (paths []string, fileName, structName string) {
	pathlist := strings.Split(fullPath, "/")
	paths = pathlist[:len(pathlist)-1]
	fileName, structName = GetFileAndStruct(pathlist[len(pathlist)-1])
	return
}

func New(items map[string]*NodeInfo, repositoryName string) *Node {
	tree := &Node{
		Name:        repositoryName,
		FullPath:    repositoryName,
		Type:        PackageType,
		childrenMap: make(map[string]*Node),
		Children:    make([]*Node, 0),
	}

	for key, value := range items {
		currentNode := tree
		paths, fileName, structName := getPathAndFile(key)
		currentPath := repositoryName
		for _, path := range paths {
			if path == "" {
				continue
			}
			nodePath := currentPath + "/" + path
			_, ok := currentNode.childrenMap[path]
			if !ok {
				currentNode.childrenMap[path] = &Node{
					Name:        path,
					FullPath:    nodePath,
					Type:        PackageType,
					childrenMap: make(map[string]*Node),
				}
			}
			currentNode = currentNode.childrenMap[path]
			currentPath = nodePath
		}
		fileNodePath := currentPath + "/" + fileName
		_, ok := currentNode.childrenMap[fileName]
		if !ok {
			currentNode.childrenMap[fileName] = &Node{
				Name:        fileName,
				FullPath:    fileNodePath,
				Type:        FileType,
				childrenMap: make(map[string]*Node),
			}
		}
		fileNode := currentNode.childrenMap[fileName]
		if len(structName) > 0 {
			structNodePath := fileNodePath + ".(" + structName + ")"
			structNode, ok := fileNode.childrenMap[structName]
			if !ok {
				fileNode.childrenMap[structName] = &Node{
					Name:               structName,
					FullPath:           structNodePath,
					Type:               StructType,
					Line:               value.Line,
					NumberOfAttributes: value.NumberAttributes,
					NumberOfMethods:    value.NumberMethods,
					NumberOfLines:      value.NumberLines,
				}
			} else {
				structNode.NumberOfAttributes += value.NumberAttributes
				structNode.NumberOfLines += value.NumberLines
				structNode.NumberOfMethods += value.NumberMethods
			}
		} else {
			fileNode.NumberOfAttributes += value.NumberAttributes
			fileNode.NumberOfLines += value.NumberLines
			fileNode.NumberOfMethods += value.NumberMethods
		}
	}
	tree.GenerateChildList()
	tree.GenerateFlatChildrenPosition()
	tree.accumulateMetrics()
	return tree
}

func (n *Node) accumulateMetrics() {
	if len(n.Children) == 0 {
		return
	}
	for _, child := range n.Children {
		child.accumulateMetrics()
		n.NumberOfLines += child.NumberOfLines
		n.NumberOfMethods += child.NumberOfMethods
		n.NumberOfAttributes += child.NumberOfAttributes
	}
}