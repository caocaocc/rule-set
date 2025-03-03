package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"
)

var (
	dataPath     = flag.String("datapath", filepath.Join("./", "data"), "Path to your custom 'data' directory")
	datName      = flag.String("datname", "geosite.dat", "Name of the generated dat file")
	outputPath   = flag.String("outputpath", "./publish", "Output path to the generated files")
	exportLists  = flag.String("exportlists", "cdn,cn,geolocation-cn,geolocation-!cn,private,apple,icloud,google,steam,bilibili,paypal,openai,netflix,tiktok,category-ai-chat-!cn,category-media", "Lists to be exported in plaintext format, separated by ',' comma")
	excludeAttrs = flag.String("excludeattrs", "cn@!cn@ads,geolocation-cn@!cn@ads,geolocation-!cn@cn@ads", "Exclude rules with certain attributes in certain lists, seperated by ',' comma, support multiple attributes in one list. Example: geolocation-!cn@cn@ads,geolocation-cn@!cn")
	toGFWList    = flag.String("togfwlist", "geolocation-!cn", "List to be exported in GFWList format")
)

func main() {
	flag.Parse()

	dir := GetDataDir()
	listInfoMap := make(ListInfoMap)

	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if err := listInfoMap.Marshal(path); err != nil {
			return err
		}
		return nil
	}); err != nil {
		fmt.Println("Failed:", err)
		os.Exit(1)
	}

	if err := listInfoMap.FlattenAndGenUniqueDomainList(); err != nil {
		fmt.Println("Failed:", err)
		os.Exit(1)
	}

	// Process and split *excludeRules
	excludeAttrsInFile := make(map[fileName]map[attribute]bool)
	if *excludeAttrs != "" {
		exFilenameAttrSlice := strings.Split(*excludeAttrs, ",")
		for _, exFilenameAttr := range exFilenameAttrSlice {
			exFilenameAttr = strings.TrimSpace(exFilenameAttr)
			exFilenameAttrMap := strings.Split(exFilenameAttr, "@")
			filename := fileName(strings.ToUpper(strings.TrimSpace(exFilenameAttrMap[0])))
			excludeAttrsInFile[filename] = make(map[attribute]bool)
			for _, attr := range exFilenameAttrMap[1:] {
				attr = strings.TrimSpace(attr)
				if len(attr) > 0 {
					excludeAttrsInFile[filename][attribute(attr)] = true
				}
			}
		}
	}

	// Process and split *exportLists
	var exportListsSlice []string
	if *exportLists != "" {
		tempSlice := strings.Split(*exportLists, ",")
		for _, exportList := range tempSlice {
			exportList = strings.TrimSpace(exportList)
			if len(exportList) > 0 {
				exportListsSlice = append(exportListsSlice, exportList)
			}
		}
	}

	// Generate dlc.dat
	if geositeList := listInfoMap.ToProto(excludeAttrsInFile); geositeList != nil {
		protoBytes, err := proto.Marshal(geositeList)
		if err != nil {
			fmt.Println("Failed:", err)
			os.Exit(1)
		}
		if err := os.MkdirAll(*outputPath, 0755); err != nil {
			fmt.Println("Failed:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(filepath.Join(*outputPath, *datName), protoBytes, 0644); err != nil {
			fmt.Println("Failed:", err)
			os.Exit(1)
		} else {
			fmt.Printf("%s has been generated successfully in '%s'.\n", *datName, *outputPath)
		}
	}

	// Generate plaintext list files
	if filePlainTextBytesMap, err := listInfoMap.ToPlainText(exportListsSlice); err == nil {
		for filename, plaintextBytes := range filePlainTextBytesMap {
			// Generate .txt files
			if err := os.WriteFile(filepath.Join(*outputPath, filename+".txt"), plaintextBytes, 0644); err != nil {
				fmt.Println("Failed:", err)
				os.Exit(1)
			} else {
				fmt.Printf("%s.txt has been generated successfully in '%s'.\n", filename, *outputPath)
			}
			
			// Generate Surge .list files
			if surgeBytes := listInfoMap[fileName(strings.ToUpper(filename))].ToSurgeList(); len(surgeBytes) > 0 {
				if err := os.WriteFile(filepath.Join(*outputPath, filename+".list"), surgeBytes, 0644); err != nil {
					fmt.Println("Failed:", err)
					os.Exit(1)
				} else {
					fmt.Printf("%s.list has been generated successfully in '%s'.\n", filename, *outputPath)
				}
			}

			// Generate Mihomo/Clash.Meta .yaml files
			if mihomoBytes := listInfoMap[fileName(strings.ToUpper(filename))].ToMihomoList(); len(mihomoBytes) > 0 {
				if err := os.WriteFile(filepath.Join(*outputPath, filename+".yaml"), mihomoBytes, 0644); err != nil {
					fmt.Println("Failed:", err)
					os.Exit(1)
				} else {
					fmt.Printf("%s.yaml has been generated successfully in '%s'.\n", filename, *outputPath)
				}
			}

			// Generate sing-box .json files
			if singboxBytes := listInfoMap[fileName(strings.ToUpper(filename))].ToSingBoxList(); len(singboxBytes) > 0 {
				if err := os.WriteFile(filepath.Join(*outputPath, filename+".json"), singboxBytes, 0644); err != nil {
					fmt.Println("Failed:", err)
					os.Exit(1)
				} else {
					fmt.Printf("%s.json has been generated successfully in '%s'.\n", filename, *outputPath)
				}
			}

			// Generate Quantumult X .snippet files
			if qxBytes := listInfoMap[fileName(strings.ToUpper(filename))].ToQuantumultXList(); len(qxBytes) > 0 {
				if err := os.WriteFile(filepath.Join(*outputPath, filename+".snippet"), qxBytes, 0644); err != nil {
					fmt.Println("Failed:", err)
					os.Exit(1)
				} else {
					fmt.Printf("%s.snippet has been generated successfully in '%s'.\n", filename, *outputPath)
				}
			}
		}
	} else {
		fmt.Println("Failed:", err)
		os.Exit(1)
	}

	// Generate gfwlist.txt
	if gfwlistBytes, err := listInfoMap.ToGFWList(*toGFWList); err == nil {
		if f, err := os.OpenFile(filepath.Join(*outputPath, "gfwlist.txt"), os.O_RDWR|os.O_CREATE, 0644); err != nil {
			fmt.Println("Failed:", err)
			os.Exit(1)
		} else {
			encoder := base64.NewEncoder(base64.StdEncoding, f)
			defer encoder.Close()
			if _, err := encoder.Write(gfwlistBytes); err != nil {
				fmt.Println("Failed:", err)
				os.Exit(1)
			}
			fmt.Printf("gfwlist.txt has been generated successfully in '%s'.\n", *outputPath)
		}
	} else {
		fmt.Println("Failed:", err)
		os.Exit(1)
	}

	// Generate ipcidr
	fmt.Println("\nGenerating IP rules...")
	
	ipSets := []*IPSet{
		NewIPSet("private", []string{
			"https://raw.githubusercontent.com/Loyalsoldier/geoip/release/text/private.txt",
		}, *outputPath),
		NewIPSet("cn", []string{
			"https://raw.githubusercontent.com/misakaio/chnroutes2/master/chnroutes.txt",
			"https://raw.githubusercontent.com/gaoyifan/china-operator-ip/ip-lists/china6.txt",
		}, *outputPath),
		NewIPSet("telegram", []string{
			"https://core.telegram.org/resources/cidr.txt",
		}, *outputPath),
	}

	policies := map[string]string{
		"private":  "direct",
		"cn":       "direct",
		"telegram": "proxy",
	}

	for _, set := range ipSets {
		if err := set.Generate(policies[set.Name]); err != nil {
			fmt.Printf("Error generating %s: %v\n", set.Name, err)
			continue
		}
		fmt.Printf("%s: %d entries\n", set.Name, len(set.IPs))
	}
}
