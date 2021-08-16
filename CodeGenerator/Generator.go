package CodeGenerator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type Content struct {
	Output  string   `json:"output"`
	Input   string   `json:"input"`
	Modules []Module `json:"layers"`
}

type Module struct {
	Category string  `json:"category"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Input    *string `json:"input"`
	Output   *string `json:"output"`
	Param    Param   `json:"param"`
}

func (m *Module) ToCode() (string, error) {
	var result string
	param, err := m.Param.ToCode(m.Type)

	if m.Input != nil {
		result += m.Name
		result += " = "
	}
	result += tf + keras + layers + "." + param
	if m.Output != nil {
		result += "(" + *m.Output + ")\n"
	}

	return result, err
}

type Project struct {
	Config  Config  `json:"config"`
	Content Content `json:"content"`
}

const (
	importTf      = "import tensorflow as tf\n\n"
	tf            = "tf"
	keras         = ".keras"
	layers        = ".layers"
	createModel   = "model = tf.keras.Model(inputs=%s, outputs=%s)\n\n"
	fitModel      = "model.fit(%s, %s, epochs=%d, batch_size=%d, validation_split=%g, callbacks=%s)\n"
	remoteMonitor = tf + keras + ".callbacks.RemoteMonitor(root=%s, path=%s field='data', header=None, send_as_json=True)\n"
)

func digitCheck(target string) bool {
	re, err := regexp.Compile("\\d")
	if err != nil {
		panic(err)
	}

	return re.MatchString(target)
}

func SortLayers(source []Module) []Module {
	// Sorting layer components via BFS.
	type node struct {
		idx    int
		Output *string
	}

	var result []Module            // result Content slice.
	adj := make(map[string][]node) // adjustment matrix of each nodes.
	var inputIdx int

	// setup adjustment matrix.
	for idx, layer := range source {
		// Input layer is always first.u
		var input string
		if layer.Type == "Input" {
			inputIdx = idx

			// result = append(result, layer)
		}
		input = layer.Name

		var nodeSlice []node
		if adj[input] == nil {
			nodeSlice = append(nodeSlice, node{idx, layer.Output})
			adj[input] = nodeSlice
		} else {
			prev, _ := adj[input]
			nodeSlice = prev
			nodeSlice = append(nodeSlice, node{idx, layer.Output})
			adj[input] = nodeSlice
		}
	}

	// Using BFS with queue
	var q Queue
	q.Push(source[inputIdx].Name)
	for !q.Empty() {
		current := q.Pop()
		for _, next := range adj[current] {
			if next.Output != nil {
				q.Push(*next.Output)
			}
			result = append(result, source[next.idx])
		}
	}

	return result
}

// Generate layer codes from content.json
func (c *Content) GenLayers() ([]string, error) {
	var codes []string

	layers := SortLayers(c.Modules)

	// code converting
	for _, d := range layers {
		layer, err := d.ToCode()
		if err != nil {
			return nil, err
		}

		codes = append(codes, layer)
	}

	// create model.
	model := fmt.Sprintf(createModel, c.Input, c.Output)
	codes = append(codes, model)

	return codes, nil
}

// generate compile codes from config.json
func (c *Config) GenConfig() ([]string, error) {
	var codes []string

	// get optimizer
	optimizer := fmt.Sprintf("%s.optimizers.%s(learning_rate=%g)", tf+keras, strings.Title(c.Optimizer), c.LearningRate)

	// get metrics
	var metrics string
	for i := 1; i <= len(c.Metrics); i++ {
		metrics += fmt.Sprintf("\"%s\"", c.Metrics[i-1])
		if i < len(c.Metrics) {
			metrics += ", "
		}
	}

	// get compile
	compile := fmt.Sprintf("model.compile(optimizer=%s, loss=\"%s\", metrics=[%s])\n", optimizer, c.Loss, metrics)
	codes = append(codes, compile)

	es, err := c.EarlyStopping.GenCode()
	if err != nil {
		return nil, err
	}
	codes = append(codes, es)

	lrr, err := c.LearningRateReduction.GenCode()
	if err != nil {
		return nil, err
	}
	codes = append(codes, lrr)

	return codes, nil
}

func (c *Config) GenFit() string {
	// callbacks
	var callbacks string
	callbacks += "["
	callbacks += fmt.Sprintf(
		remoteMonitor,
		"http://localohst:8080",
		"/publish/epoch/end",
	)
	if *c.LearningRateReduction.Usage {
		callbacks += ", learning_rate_reduction"
	}
	if *c.EarlyStopping.Usage {
		callbacks += ", early_stop"
	}
	callbacks += "]"

	code := fmt.Sprintf(
		fitModel,
		"data",
		"label",
		c.Epochs,
		c.BatchSize,
		0.3,
		callbacks,
	)

	return code
}

func GenerateModel(config Config, content Content, fit bool) error {
	// @ param fit
	//	true => remote train.
	//	false => extract python code from graph.

	var codes []string
	codes = append(codes, importTf)

	Layers, err := content.GenLayers()
	if err != nil {
		return err
	}
	codes = append(codes, Layers...)

	Configs, err := config.GenConfig()
	if err != nil {
		return err
	}
	codes = append(codes, Configs...)

	// if request is remote train, add fit code
	if fit {
		codes = append(codes, config.GenFit())
	}

	// create python file
	py, err := os.Create("model.py")
	if err != nil {
		return err
	}
	defer py.Close()

	fileSize := 0
	for _, code := range codes {
		n, err := py.Write([]byte(code))
		if err != nil {
			return err
		}
		fileSize += n
	}

	fmt.Printf("Code converting is finish with %d bytes size\n", fileSize)

	return nil
}

func BindProject(r *http.Request) (*Project, error) {
	project := new(Project)
	data := make(map[string]json.RawMessage)
	cc := make(map[string]json.RawMessage)
	var layers []map[string]json.RawMessage

	// Binding request body
	err := json.NewDecoder(r.Body).Decode(&data)

	if err != nil {
		return nil, err
	}

	// Unmarshalling Config.
	err = json.Unmarshal(data["config"], &project.Config)
	if err != nil {
		return nil, err
	}

	// Unmarshalling Content.
	err = json.Unmarshal(data["content"], &cc)
	if err != nil {
		return nil, err
	}

	// Unmarshalling content input and output.
	err = json.Unmarshal(cc["input"], &project.Content.Input)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(cc["output"], &project.Content.Output)
	if err != nil {
		return nil, err
	}

	// Unmarshalling Modules.
	err = json.Unmarshal(cc["layers"], &layers)

	for _, layer := range layers {
		// Unmarshalling module informations except parameters.
		mod, err := UnmarshalModule(layer)
		if err != nil {
			return nil, err
		}
		switch mod.Type {
		case "Conv2D":
			err = json.Unmarshal(layer["param"], &mod.Param.Conv2D)
			if err != nil {
				return nil, err
			}
		case "Dense":
			err = json.Unmarshal(layer["param"], &mod.Param.Dense)
			if err != nil {
				return nil, err
			}
		case "AveragePooling2D":
			err = json.Unmarshal(layer["param"], &mod.Param.AveragePooling2D)
			if err != nil {
				return nil, err
			}
		case "MaxPool2D":
			err = json.Unmarshal(layer["param"], &mod.Param.MaxPool2D)
			if err != nil {
				return nil, err
			}
		case "Activation":
			err = json.Unmarshal(layer["param"], &mod.Param.Activation)
			if err != nil {
				return nil, err
			}
		case "Dropout":
			err = json.Unmarshal(layer["param"], &mod.Param.Dropout)
			if err != nil {
				return nil, err
			}
		case "BatchNormalization":
			err = json.Unmarshal(layer["param"], &mod.Param.BatchNormalization)
			if err != nil {
				return nil, err
			}
		case "Flatten":
			err = json.Unmarshal(layer["param"], &mod.Param.Flatten)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("inavlid node type")
		}
		project.Content.Modules = append(project.Content.Modules, mod)
	}

	return project, nil
}
