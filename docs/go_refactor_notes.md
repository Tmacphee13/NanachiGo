# Notes about Go in the Mindmap Context

This is a document meant to log some information about go an how to leverage go as a backend to a webapp like the Research Paper Mindmap

## Getting Started

**Refactoring JS Backend**

I leveraged LLMs to convert the existing Javascript server.js file to go. To capture the imports/dependencies used in my module, I used `go mod init` and `go mod tidy` to create and update the go.mod file. It scans the project, updates go.mod, and downloads all dependencies mentioned in go.mod.

To build the module, we use `go build` which will compile the code and build an executable, but will do that once the rest of the code has been refactored

**Go and JS**

We should not (but maybe could?) build both the front end and backend in go. The front end will continue to be JS and html while the backend will be go

*Backend (Go): Handles server-side logic, such as:*

* API endpoints (e.g., /api/mindmaps, /api/upload).
* Business logic and database interactions (e.g., DynamoDB or other databases).
* Integration with external services (e.g., AWS Bedrock, Claude).
* Serving frontend files (like index.html, script.js, and style.css).

*Frontend (JavaScript): Handles client-side logic, such as:

* User interface (UI) rendering and interactions.
* Fetching data from the backend API and updating the UI dynamically.
* Managing state and user input.

## Project Structure

.
├── main.go                # Main entry point for the Go backend
├── handlers/              # Contains all handler functions for API routes
│   ├── upload.go          # `uploadHandler`
│   ├── mindmap.go         # `getMindmapsHandler`, `deleteMindmapHandler`
│   ├── redo_description.go # `redoDescriptionHandler`
├── models/                # Database models and logic
│   ├── mindmap.go         # Mindmap struct and DynamoDB functions
├── utils/                 # Utility functions and shared logic
│   ├── claude.go          # `callClaude` and related functions
│   ├── helpers.go         # Utility functions like `updateNodeByPath`
├── public/                # Frontend resources (static files)
│   ├── index.html         # Main HTML file
│   ├── style.css          # CSS for styling
│   ├── script.js          # JavaScript for client-side logic (e.g., D3.js)
│   ├── assets/            # Optional: Images, fonts, or other static assets
├── go.mod                 # Go module file for dependency management
├── go.sum                 # Go module checksum file
