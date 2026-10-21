package strategies

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"heatmetro-go/internal/domain/metrics"
	"heatmetro-go/internal/domain/project"
)

type JavaStrategy struct{}

func NewJavaStrategy() *JavaStrategy {
	return &JavaStrategy{}
}

var javaKeywords = map[string]bool{
	"abstract": true, "assert": true, "boolean": true, "break": true, "byte": true, "case": true,
	"catch": true, "char": true, "class": true, "const": true, "continue": true, "default": true,
	"do": true, "double": true, "else": true, "enum": true, "extends": true, "final": true,
	"finally": true, "float": true, "for": true, "goto": true, "if": true, "implements": true,
	"import": true, "instanceof": true, "int": true, "interface": true, "long": true, "native": true,
	"new": true, "package": true, "private": true, "protected": true, "public": true, "return": true,
	"short": true, "static": true, "strictfp": true, "super": true, "switch": true, "synchronized": true,
	"this": true, "throw": true, "throws": true, "transient": true, "try": true, "void": true,
	"volatile": true, "while": true, "true": true, "false": true, "null": true,
}

var regexJavaTokens = regexp.MustCompile(`[a-zA-Z_$][a-zA-Z0-9_$]*|\d+(\.\d+)?|[+\-*/%=!&|^~<>?:]+|[(){}\[\],.;]`)

func (s *JavaStrategy) CalculateHalstead(proj project.Project) (map[string]metrics.Halstead, error) {
	results := make(map[string]metrics.Halstead)
	fmt.Printf("--- Calculando Halstead Java (Regex) en: %s ---\n", proj.Path)

	filepath.WalkDir(proj.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".java") {
			return nil
		}
		relPath, _ := filepath.Rel(proj.Path, path)
		relPath = strings.ReplaceAll(relPath, "\\", "/")
		if isExcluded(relPath, proj.Exclusions) {
			return nil
		}

		content, _ := os.ReadFile(path)
		cleanCode := removeJavaComments(string(content))

		// Contar Literales
		strRegex := regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
		charRegex := regexp.MustCompile(`'(?:[^'\\]|\\.)*'`)
		stringLiterals := len(strRegex.FindAllString(cleanCode, -1))
		charLiterals := len(charRegex.FindAllString(cleanCode, -1))

		cleanCode = strRegex.ReplaceAllString(cleanCode, "")
		cleanCode = charRegex.ReplaceAllString(cleanCode, "")

		tokens := regexJavaTokens.FindAllString(cleanCode, -1)

		operators := make(map[string]int)
		operands := make(map[string]int)
		totalOperators := 0
		totalOperands := 0

		for _, token := range tokens {
			if javaKeywords[token] {
				operators[token]++
				totalOperators++
			} else if regexp.MustCompile(`^[a-zA-Z_$]`).MatchString(token) || regexp.MustCompile(`^\d`).MatchString(token) {
				operands[token]++
				totalOperands++
			} else {
				operators[token]++
				totalOperators++
			}
		}

		totalOperandsCount := totalOperands + stringLiterals + charLiterals
		n1 := float64(len(operators))
		n2 := float64(len(operands))
		if stringLiterals > 0 { n2++ }
		if charLiterals > 0 { n2++ }

		N1 := float64(totalOperators)
		N2 := float64(totalOperandsCount)

		var vol, diff, eff, bugs float64
		if n1 > 0 && n2 > 0 {
			n := n1 + n2
			N := N1 + N2
			vol = N * math.Log2(n)
			diff = (n1 / 2.0) * (N2 / n2)
			eff = diff * vol
			bugs = vol / 3000.0
		}

		results[relPath] = metrics.Halstead{
			Volume: math.Round(vol*100)/100, Difficulty: math.Round(diff*100)/100,
			Effort: math.Round(eff*100)/100, Bugs: math.Round(bugs*100)/100,
		}
		return nil
	})

	return results, nil
}

func (s *JavaStrategy) CalculateCoupling(proj project.Project) (map[string]metrics.Coupling, error) {
	globalImportCounts := make(map[string]int)
	packageImportCounts := make(map[string]int)
	
	type fileRegistry struct {
		relPath     string
		fullName    string
		packageName string
		fanOut      int
	}
	var registry []fileRegistry

	fmt.Printf("--- Calculando Coupling Java en: %s ---\n", proj.Path)

	filepath.WalkDir(proj.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".java") { return nil }
		
		relPath, _ := filepath.Rel(proj.Path, path)
		relPath = strings.ReplaceAll(relPath, "\\", "/")
		if isExcluded(relPath, proj.Exclusions) { return nil }

		content, _ := os.ReadFile(path)
		cleanCode := removeJavaComments(string(content))

		// 1. Paquete y Clase
		pkgRegex := regexp.MustCompile(`(?m)^\s*package\s+([\w\.]+)\s*;`)
		pkgMatch := pkgRegex.FindStringSubmatch(cleanCode)
		packageName := ""
		if len(pkgMatch) > 1 {
			packageName = pkgMatch[1]
		}
		className := strings.TrimSuffix(filepath.Base(path), ".java")
		myFullName := className
		if packageName != "" {
			myFullName = packageName + "." + className
		}

		// 2. Imports
		importRegex := regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([\w\.\*]+)\s*;`)
		importMatches := importRegex.FindAllStringSubmatch(cleanCode, -1)
		
		fanOut := len(importMatches)
		for _, m := range importMatches {
			imp := m[1]
			if !strings.HasSuffix(imp, "*") {
				globalImportCounts[imp]++
			} else {
				pkg := imp[:len(imp)-2]
				packageImportCounts[pkg]++
			}
		}

		registry = append(registry, fileRegistry{relPath, myFullName, packageName, fanOut})
		return nil
	})

	results := make(map[string]metrics.Coupling)
	for _, f := range registry {
		fanIn := globalImportCounts[f.fullName]
		if f.packageName != "" {
			fanIn += packageImportCounts[f.packageName]
		}
		results[f.relPath] = metrics.Coupling{NumDependency: fanIn, NumImports: f.fanOut}
	}
	return results, nil
}


// El algoritmo 3D Generator
type Generator struct {
	Dimension, currentIndex        int
	xReference, yReference         float64
	maxWidth, maxHeight            float64
	defaultMargin                  float64
}

func NewGenerator(numberNodes int) *Generator {
	return &Generator{
		Dimension: int(math.Ceil(math.Sqrt(float64(numberNodes)))),
		defaultMargin: 1.0,
	}
}

func (g *Generator) GetBounds() (float64, float64) {
	return g.maxWidth + g.defaultMargin, g.maxHeight + g.defaultMargin
}

func (g *Generator) NextPosition(width, height float64) (float64, float64) {
	g.currentIndex++
	if g.currentIndex > g.Dimension && g.yReference+height >= g.maxWidth {
		g.currentIndex = 0
		g.yReference = 0
		g.xReference = g.maxWidth + g.defaultMargin
	}
	x := g.xReference + (width+g.defaultMargin)/2.0
	y := g.yReference + (height+g.defaultMargin)/2.0
	if g.xReference+width > g.maxWidth { g.maxWidth = g.xReference + width }
	if g.yReference+height > g.maxHeight { g.maxHeight = g.yReference + height }
	g.yReference += height + g.defaultMargin
	return x, y
}

// El AST Node
type Node struct {
	Name, Type, FullPath string
	Width, Depth, ChildWidth, ChildDepth, ChildX, ChildY float64
	NumberOfLines, NumberOfMethods, NumberOfAttributes int
	Children []*Node
	ChildrenMap map[string]*Node
}

func NewNode(name, typ, fullPath string) *Node {
	return &Node{Name: name, Type: typ, FullPath: fullPath, ChildrenMap: make(map[string]*Node)}
}

func (n *Node) AccumulateMetrics() {
	for _, child := range n.Children {
		child.AccumulateMetrics()
		n.NumberOfLines += child.NumberOfLines
		n.NumberOfMethods += child.NumberOfMethods
		n.NumberOfAttributes += child.NumberOfAttributes
	}
}

func (n *Node) GenerateFlatPosition() {
	if n.Type == "STRUCT" || len(n.Children) == 0 {
		size := float64(n.NumberOfAttributes + 1)
		n.Width, n.Depth, n.ChildWidth, n.ChildDepth = size, size, size, size
		return
	}

	for _, child := range n.Children {
		child.GenerateFlatPosition()
		if child.Type == "PACKAGE" {
			child.ChildWidth, child.ChildDepth = 5.0, 5.0
		}
	}

	// Ordenamiento por tamaño (Equivalente al Sort.go nativo)
	sort.Slice(n.Children, func(i, j int) bool {
		if n.Children[i].ChildWidth != n.Children[j].ChildWidth {
			return n.Children[i].ChildWidth > n.Children[j].ChildWidth
		}
		return n.Children[i].Name < n.Children[j].Name
	})

	gen := NewGenerator(len(n.Children))
	for _, child := range n.Children {
		child.ChildX, child.ChildY = gen.NextPosition(child.ChildWidth, child.ChildDepth)
	}

	n.Width, n.Depth = gen.GetBounds()
	for _, child := range n.Children {
		child.ChildX -= n.Width / 2.0
		child.ChildY -= n.Depth / 2.0
	}

	if n.Type == "FILE" {
		n.Width += float64(n.NumberOfAttributes)
		n.Depth += float64(n.NumberOfAttributes)
	}
	n.ChildWidth, n.ChildDepth = n.Width, n.Depth
}


type javaClass struct {
	name string
	loc, methods, attrs int
}

// 1. MODIFICADO: parseJavaFileAST para replicar exactamente el comportamiento de JS
func parseJavaFileAST(content string, fileName string) []javaClass {
	cleanCode := removeJavaComments(content)
	var classes []javaClass

	classPattern := regexp.MustCompile(`\b(class|interface|enum|record)\s+([A-Z]\w*)`)
	attrPattern := regexp.MustCompile(`\b(?:public|private|protected|static|final|volatile|transient)\s+[\w<>[\]]+\s+\w+(\s*=.*?)?;`)
	
	// CORRECCIÓN: Revertimos al Regex idéntico al de JS. 
	// El `|\s` final es vital para atrapar métodos package-private (como los test de JUnit).
	methodPattern := regexp.MustCompile(`(?:public|private|protected|static|final|native|synchronized|abstract|\s)+[\w\<\>\[\]]+\s+\w+\s*\(.*?\)\s*(?:throws\s+[\w,\s]+\s*)?\{`)

	matches := classPattern.FindAllStringSubmatchIndex(cleanCode, -1)
	for _, match := range matches {
		className := cleanCode[match[4]:match[5]]
		startIndex := match[0]

		classBody := extractBlockBraces(cleanCode, startIndex)
		if classBody == "" { continue }

		bodyContent := classBody
		if idx := strings.Index(classBody, "{"); idx != -1 {
			bodyContent = classBody[idx+1 : len(classBody)-1]
		}

		// CORRECCIÓN CRÍTICA: LIMPIEZA DE CLASES ANIDADAS (Step C de Node.js)
		// Borra el código de clases internas para no sumar sus métodos a la clase padre
		for {
			nestedLoc := classPattern.FindStringIndex(bodyContent)
			if nestedLoc == nil {
				break
			}
			nestedBlock := extractBlockBraces(bodyContent, nestedLoc[0])
			if nestedBlock != "" {
				bodyContent = strings.Replace(bodyContent, nestedBlock, "", 1)
			} else {
				break
			}
		}

		lines := strings.Split(classBody, "\n")
		loc := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				loc++
			}
		}

		methods := len(methodPattern.FindAllString(bodyContent, -1))
		
		fieldsOnlyContent := removeJavaMethodBodies(bodyContent)
		attrs := len(attrPattern.FindAllString(fieldsOnlyContent, -1))

		classes = append(classes, javaClass{className, loc, methods, attrs})
	}

	if len(classes) == 0 {
		lines := strings.Split(cleanCode, "\n")
		loc := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				loc++
			}
		}
		classes = append(classes, javaClass{strings.TrimSuffix(fileName, ".java"), loc, 0, 0})
	}
	return classes
}

func (s *JavaStrategy) CalculateLayout(proj project.Project) (map[string]metrics.Layout, map[string]metrics.Cohesion, error) {
	fmt.Printf("--- Calculando City Layout Java en: %s ---\n", proj.Path)
	layoutMap := make(map[string]metrics.Layout)
	cohesionMap := make(map[string]metrics.Cohesion)
	
	projectName := filepath.Base(proj.Path)
	rootNode := NewNode(projectName, "PACKAGE", projectName)

	filepath.WalkDir(proj.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".java") { return nil }
		relPath, _ := filepath.Rel(proj.Path, path)
		relPath = strings.ReplaceAll(relPath, "\\", "/")
		if isExcluded(relPath, proj.Exclusions) { return nil }

		content, _ := os.ReadFile(path)
		classes := parseJavaFileAST(string(content), filepath.Base(path))

		pathParts := strings.Split(relPath, "/")
		fileName := pathParts[len(pathParts)-1]
		folderParts := pathParts[:len(pathParts)-1]

		currentNode := rootNode
		currentPath := rootNode.FullPath
		for _, part := range folderParts {
			currentPath += "/" + part
			if currentNode.ChildrenMap[part] == nil {
				newPkg := NewNode(part, "PACKAGE", currentPath)
				currentNode.ChildrenMap[part] = newPkg
				currentNode.Children = append(currentNode.Children, newPkg)
			}
			currentNode = currentNode.ChildrenMap[part]
		}

		fileNodePath := currentPath + "/" + fileName
		if currentNode.ChildrenMap[fileName] == nil {
			newFile := NewNode(fileName, "FILE", fileNodePath)
			currentNode.ChildrenMap[fileName] = newFile
			currentNode.Children = append(currentNode.Children, newFile)
		}
		currentNode = currentNode.ChildrenMap[fileName]

		for _, cls := range classes {
			structPath := fileNodePath + ".(" + cls.name + ")"
			if currentNode.ChildrenMap[cls.name] == nil {
				newStruct := NewNode(cls.name, "STRUCT", structPath)
				newStruct.NumberOfLines = cls.loc
				newStruct.NumberOfMethods = cls.methods
				newStruct.NumberOfAttributes = cls.attrs
				currentNode.ChildrenMap[cls.name] = newStruct
				currentNode.Children = append(currentNode.Children, newStruct)
			}
		}
		return nil
	})

	rootNode.AccumulateMetrics()
	rootNode.GenerateFlatPosition()
	
	// CORRECCIÓN: Ajustamos el prefijo para la recursión, debe ser exactamente igual que en Node.js ("Proyecto/")
	flattenJavaTree(rootNode, rootNode.FullPath+"/", layoutMap, cohesionMap)

	return layoutMap, cohesionMap, nil
}

func flattenJavaTree(n *Node, prefix string, lMap map[string]metrics.Layout, cMap map[string]metrics.Cohesion) {
	// 1. Procesar el Root Node (Equivalente al inicio de flattenToObject en JS)
	if n.FullPath == strings.TrimSuffix(prefix, "/") { // Es el root node exacto
		lMap["/"] = metrics.Layout{
			RootW: n.Width, RootD: n.Depth, 
			ChildW: 0.0, ChildD: 0.0,  // <-- CORRECCIÓN: JS fuerza child=0 para la raíz
			ChildX: 0.0, ChildY: 0.0,
		}
		cMap["/"] = metrics.Cohesion{
			Type: n.Type, LOC: n.NumberOfLines, MethodCount: n.NumberOfMethods, AttrCount: n.NumberOfAttributes,
		}
	} else {
		// Cálculo del path relativo (Display Path)
		displayPath := n.FullPath
		displayPath = strings.TrimPrefix(displayPath, prefix)

		lMap[displayPath] = metrics.Layout{
			RootW: n.Width, RootD: n.Depth, ChildW: n.ChildWidth, ChildD: n.ChildDepth,
			ChildX: n.ChildX, ChildY: n.ChildY,
		}
		cMap[displayPath] = metrics.Cohesion{
			Type: n.Type, LOC: n.NumberOfLines, MethodCount: n.NumberOfMethods, AttrCount: n.NumberOfAttributes,
		}
	}

	for _, child := range n.Children {
		// En JS, solo si tiene hijos llama de forma recursiva (esto ya lo hace implicitamente el for)
		flattenJavaTree(child, prefix, lMap, cMap)
	}
}

func removeJavaComments(code string) string {
	reMultiline := regexp.MustCompile(`(?s)/\*.*?\*/`)
	code = reMultiline.ReplaceAllString(code, "")
	reSingleline := regexp.MustCompile(`//.*`)
	return reSingleline.ReplaceAllString(code, "")
}

func extractBlockBraces(text string, startIndex int) string {
	openBraces := 0
	started := false
	for i := startIndex; i < len(text); i++ {
		if text[i] == '{' {
			openBraces++
			started = true
		} else if text[i] == '}' {
			openBraces--
		}
		if started && openBraces == 0 {
			return text[startIndex : i+1]
		}
	}
	return text[startIndex:] 
}

func removeJavaMethodBodies(code string) string {
	var result strings.Builder
	i := 0
	for i < len(code) {
		if code[i] == '{' {
			openBraces := 1
			i++
			for i < len(code) && openBraces > 0 {
				if code[i] == '{' { openBraces++ }
				if code[i] == '}' { openBraces-- }
				i++
			}
			result.WriteString(";")
		} else {
			result.WriteByte(code[i])
			i++
		}
	}
	return result.String()
}