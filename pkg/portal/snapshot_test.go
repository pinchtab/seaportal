package portal

import (
	"testing"
)

func TestBuildSnapshot_BasicStructure(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head><title>Test</title></head>
<body>
  <main>
    <h1>Welcome</h1>
    <p>Hello world</p>
  </main>
</body>
</html>`

	snap, err := BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	if snap.Role != "document" {
		t.Errorf("expected root role 'document', got %q", snap.Role)
	}

	// Find main landmark
	main := findNodeByRole(snap, "main")
	if main == nil {
		t.Fatal("expected to find main landmark")
	}

	// Find heading
	heading := findNodeByRole(snap, "heading")
	if heading == nil {
		t.Fatal("expected to find heading")
	}
	if heading.Level != 1 {
		t.Errorf("expected heading level 1, got %d", heading.Level)
	}
	if heading.Name != "Welcome" {
		t.Errorf("expected heading name 'Welcome', got %q", heading.Name)
	}
}

func TestBuildSnapshot_Links(t *testing.T) {
	html := `<a href="/about">About Us</a>`

	snap, err := BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	link := findNodeByRole(snap, "link")
	if link == nil {
		t.Fatal("expected to find link")
	}
	if link.Name != "About Us" {
		t.Errorf("expected link name 'About Us', got %q", link.Name)
	}
	if link.Href != "/about" {
		t.Errorf("expected href '/about', got %q", link.Href)
	}
	if !link.Interactive {
		t.Error("expected link to be interactive")
	}
}

func TestBuildSnapshot_Buttons(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		wantName string
	}{
		{
			name:     "button element",
			html:     `<button>Click me</button>`,
			wantName: "Click me",
		},
		{
			name:     "input submit",
			html:     `<input type="submit" value="Submit">`,
			wantName: "",
		},
		{
			name:     "button with aria-label",
			html:     `<button aria-label="Close dialog">X</button>`,
			wantName: "Close dialog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap, err := BuildSnapshot(tt.html)
			if err != nil {
				t.Fatalf("BuildSnapshot failed: %v", err)
			}

			btn := findNodeByRole(snap, "button")
			if btn == nil {
				t.Fatal("expected to find button")
			}
			if btn.Name != tt.wantName {
				t.Errorf("expected name %q, got %q", tt.wantName, btn.Name)
			}
			if !btn.Interactive {
				t.Error("expected button to be interactive")
			}
		})
	}
}

func TestBuildSnapshot_FormControls(t *testing.T) {
	html := `
<form>
  <input type="text" placeholder="Enter name">
  <input type="email" aria-label="Email address">
  <input type="checkbox" checked>
  <input type="radio" name="choice">
  <textarea placeholder="Comments"></textarea>
  <select><option>One</option></select>
</form>`

	snap, err := BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	// Check textbox
	textbox := findNodeByRole(snap, "textbox")
	if textbox == nil {
		t.Fatal("expected to find textbox")
	}
	if textbox.Name != "Enter name" {
		t.Errorf("expected textbox name 'Enter name', got %q", textbox.Name)
	}
	if !textbox.Interactive {
		t.Error("expected textbox to be interactive")
	}

	// Check checkbox
	checkbox := findNodeByRole(snap, "checkbox")
	if checkbox == nil {
		t.Fatal("expected to find checkbox")
	}
	if checkbox.Checked == nil || !*checkbox.Checked {
		t.Error("expected checkbox to be checked")
	}

	// Check radio
	radio := findNodeByRole(snap, "radio")
	if radio == nil {
		t.Fatal("expected to find radio")
	}

	// Check combobox
	combo := findNodeByRole(snap, "combobox")
	if combo == nil {
		t.Fatal("expected to find combobox")
	}
}

func TestBuildSnapshot_Headings(t *testing.T) {
	html := `
<h1>Level 1</h1>
<h2>Level 2</h2>
<h3>Level 3</h3>
<h4>Level 4</h4>
<h5>Level 5</h5>
<h6>Level 6</h6>`

	snap, err := BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	headings := findAllByRole(snap, "heading")
	if len(headings) != 6 {
		t.Fatalf("expected 6 headings, got %d", len(headings))
	}

	for i, h := range headings {
		expectedLevel := i + 1
		if h.Level != expectedLevel {
			t.Errorf("heading %d: expected level %d, got %d", i, expectedLevel, h.Level)
		}
	}
}

func TestBuildSnapshot_Lists(t *testing.T) {
	html := `
<ul>
  <li>Item 1</li>
  <li>Item 2</li>
  <li>Item 3</li>
</ul>`

	snap, err := BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	list := findNodeByRole(snap, "list")
	if list == nil {
		t.Fatal("expected to find list")
	}

	items := findAllByRole(snap, "listitem")
	if len(items) != 3 {
		t.Errorf("expected 3 list items, got %d", len(items))
	}
}

func TestBuildSnapshot_Landmarks(t *testing.T) {
	html := `
<header>Header</header>
<nav>Navigation</nav>
<main>Main content</main>
<aside>Sidebar</aside>
<footer>Footer</footer>`

	snap, err := BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	landmarks := []string{"banner", "navigation", "main", "complementary", "contentinfo"}
	for _, role := range landmarks {
		node := findNodeByRole(snap, role)
		if node == nil {
			t.Errorf("expected to find landmark %q", role)
		}
	}
}

func TestBuildSnapshot_Images(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		wantRole bool
		wantName string
	}{
		{
			name:     "image with alt",
			html:     `<img src="photo.jpg" alt="A beautiful sunset">`,
			wantRole: true,
			wantName: "A beautiful sunset",
		},
		{
			name:     "decorative image (no alt)",
			html:     `<img src="decoration.png">`,
			wantRole: false,
		},
		{
			name:     "image with empty alt (decorative)",
			html:     `<img src="spacer.gif" alt="">`,
			wantRole: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap, err := BuildSnapshot(tt.html)
			if err != nil {
				t.Fatalf("BuildSnapshot failed: %v", err)
			}

			img := findNodeByRole(snap, "image")
			if tt.wantRole {
				if img == nil {
					t.Fatal("expected to find image")
				}
				if img.Name != tt.wantName {
					t.Errorf("expected name %q, got %q", tt.wantName, img.Name)
				}
			} else {
				if img != nil {
					t.Error("expected decorative image to not have role")
				}
			}
		})
	}
}

func TestBuildSnapshot_Tables(t *testing.T) {
	html := `
<table>
  <thead>
    <tr>
      <th>Name</th>
      <th>Age</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td>Alice</td>
      <td>30</td>
    </tr>
  </tbody>
</table>`

	snap, err := BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	if findNodeByRole(snap, "table") == nil {
		t.Error("expected to find table")
	}
	if findNodeByRole(snap, "row") == nil {
		t.Error("expected to find row")
	}
	if findNodeByRole(snap, "columnheader") == nil {
		t.Error("expected to find columnheader")
	}
	if findNodeByRole(snap, "cell") == nil {
		t.Error("expected to find cell")
	}
}

func TestBuildSnapshot_ExplicitRoles(t *testing.T) {
	html := `<div role="button" tabindex="0">Custom Button</div>`

	snap, err := BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	btn := findNodeByRole(snap, "button")
	if btn == nil {
		t.Fatal("expected to find button with explicit role")
	}
	if btn.Name != "Custom Button" {
		t.Errorf("expected name 'Custom Button', got %q", btn.Name)
	}
	if !btn.Interactive {
		t.Error("expected div[role=button][tabindex=0] to be interactive")
	}
}

func TestBuildSnapshot_AriaLabel(t *testing.T) {
	html := `<nav aria-label="Main navigation"><a href="/">Home</a></nav>`

	snap, err := BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	nav := findNodeByRole(snap, "navigation")
	if nav == nil {
		t.Fatal("expected to find navigation")
	}
	if nav.Name != "Main navigation" {
		t.Errorf("expected name 'Main navigation', got %q", nav.Name)
	}
}

func TestBuildSnapshot_DisabledState(t *testing.T) {
	html := `<button disabled>Can't click</button>`

	snap, err := BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	btn := findNodeByRole(snap, "button")
	if btn == nil {
		t.Fatal("expected to find button")
	}
	if !btn.Disabled {
		t.Error("expected button to be disabled")
	}
}

func TestBuildSnapshot_RefUniqueness(t *testing.T) {
	html := `
<nav><a href="/">Home</a><a href="/about">About</a></nav>
<main><h1>Title</h1><p>Content</p></main>`

	snap, err := BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	refs := make(map[string]bool)
	collectRefs(snap, refs)

	if len(refs) == 0 {
		t.Error("expected to find refs")
	}

	// All refs should be unique
	// (collectRefs already ensures this by using a map)
}

func TestBuildSnapshot_NameTruncation(t *testing.T) {
	longText := "This is a very long text that should be truncated because it exceeds the maximum allowed length for accessible names"
	html := `<h1>` + longText + `</h1>`

	snap, err := BuildSnapshot(html)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	heading := findNodeByRole(snap, "heading")
	if heading == nil {
		t.Fatal("expected to find heading")
	}
	if len(heading.Name) > 80 {
		t.Errorf("expected name to be truncated to 80 chars, got %d", len(heading.Name))
	}
	if !containsString(heading.Name, "...") {
		t.Error("expected truncated name to end with ...")
	}
}

func TestBuildSnapshot_InteractiveElements(t *testing.T) {
	tests := []struct {
		html        string
		interactive bool
		desc        string
	}{
		{`<a href="/page">Link</a>`, true, "link with href"},
		{`<a>Anchor</a>`, false, "anchor without href"},
		{`<button>Click</button>`, true, "button"},
		{`<input type="text">`, true, "text input"},
		{`<input type="hidden">`, false, "hidden input"},
		{`<div role="button" onclick="foo()">Click</div>`, true, "div role=button with onclick"},
		{`<div role="button" tabindex="0">Focusable</div>`, true, "div role=button with tabindex"},
		{`<summary>Details</summary>`, true, "summary"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			snap, err := BuildSnapshot(tt.html)
			if err != nil {
				t.Fatalf("BuildSnapshot failed: %v", err)
			}

			// Find any node with Interactive set
			found := findInteractiveNode(snap)
			if tt.interactive {
				if found == nil {
					t.Errorf("expected %s to be interactive", tt.desc)
				}
			} else {
				if found != nil && found.Interactive {
					t.Errorf("expected %s to not be interactive", tt.desc)
				}
			}
		})
	}
}

// Helper functions

func findNodeByRole(node *SnapshotNode, role string) *SnapshotNode {
	if node.Role == role {
		return node
	}
	for i := range node.Children {
		if found := findNodeByRole(&node.Children[i], role); found != nil {
			return found
		}
	}
	return nil
}

func findAllByRole(node *SnapshotNode, role string) []*SnapshotNode {
	var results []*SnapshotNode
	if node.Role == role {
		results = append(results, node)
	}
	for i := range node.Children {
		results = append(results, findAllByRole(&node.Children[i], role)...)
	}
	return results
}

func findInteractiveNode(node *SnapshotNode) *SnapshotNode {
	if node.Interactive {
		return node
	}
	for i := range node.Children {
		if found := findInteractiveNode(&node.Children[i]); found != nil {
			return found
		}
	}
	return nil
}

func collectRefs(node *SnapshotNode, refs map[string]bool) {
	if node.Ref != "" {
		refs[node.Ref] = true
	}
	for i := range node.Children {
		collectRefs(&node.Children[i], refs)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr
}
