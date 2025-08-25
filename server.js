// server.js
// Backend server using Node.js and Express with AWS Bedrock Claude

import express from 'express';
import multer from 'multer';
import pdf from 'pdf-parse';
import path from 'path';
import 'dotenv/config';
import { fileURLToPath } from 'url';
import { BedrockRuntimeClient, InvokeModelCommand } from '@aws-sdk/client-bedrock-runtime';
import { DynamoDBClient } from '@aws-sdk/client-dynamodb';
import { DynamoDBDocumentClient, PutCommand, GetCommand, DeleteCommand, ScanCommand, UpdateCommand } from '@aws-sdk/lib-dynamodb';
import { v4 as uuidv4 } from 'uuid';

// --- CONFIGURATION ---
const PORT = process.env.PORT;
const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD;
const AWS_REGION = process.env.AWS_REGION;
const MINDMAPS_TABLE = process.env.MINDMAPS_TABLE;

// --- INITIALIZATION ---
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();
app.use(express.json());
app.use(express.static(path.join(__dirname, 'public')));

// --- AWS Clients ---
// Configure credentials based on environment
const awsConfig = {
  region: AWS_REGION,
};

// If using temporary credentials with session token
if (process.env.AWS_SESSION_TOKEN) {
  awsConfig.credentials = {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID,
    secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY,
    sessionToken: process.env.AWS_SESSION_TOKEN,
  };
}
const bedrockClient = new BedrockRuntimeClient(awsConfig);
const dynamoClient = new DynamoDBClient(awsConfig);
const docClient = DynamoDBDocumentClient.from(dynamoClient);

// Multer setup for file uploads in memory
const upload = multer({ storage: multer.memoryStorage() });

// --- DYNAMODB HELPER FUNCTIONS ---

/**
 * Creates a new mindmap in DynamoDB
 * @param {object} mindmapData - The mindmap data to store
 * @returns {Promise<string>} - The ID of the created mindmap
 */
async function createMindmap(mindmapData) {
  const id = uuidv4();
  const timestamp = new Date().toISOString();
  
  const item = {
    id,
    ...mindmapData,
    createdAt: timestamp,
    updatedAt: timestamp
  };

  await docClient.send(new PutCommand({
    TableName: MINDMAPS_TABLE,
    Item: item
  }));

  return id;
}

/**
 * Gets a mindmap by ID from DynamoDB
 * @param {string} id - The mindmap ID
 * @returns {Promise<object|null>} - The mindmap data or null if not found
 */
async function getMindmap(id) {
  const result = await docClient.send(new GetCommand({
    TableName: MINDMAPS_TABLE,
    Key: { id }
  }));

  return result.Item || null;
}

/**
 * Gets all mindmaps from DynamoDB, sorted by creation date
 * @returns {Promise<Array>} - Array of mindmap objects
 */
async function getAllMindmaps() {
  const result = await docClient.send(new ScanCommand({
    TableName: MINDMAPS_TABLE
  }));

  // Sort by createdAt in descending order (newest first)
  return (result.Items || []).sort((a, b) => 
    new Date(b.createdAt) - new Date(a.createdAt)
  );
}

/**
 * Updates a mindmap in DynamoDB
 * @param {string} id - The mindmap ID
 * @param {object} updates - The fields to update
 * @returns {Promise<void>}
 */
async function updateMindmap(id, updates) {
  const updateExpression = [];
  const expressionAttributeNames = {};
  const expressionAttributeValues = {};

  // Add updatedAt timestamp
  updates.updatedAt = new Date().toISOString();

  Object.keys(updates).forEach((key, index) => {
    const attrName = `#attr${index}`;
    const attrValue = `:val${index}`;
    
    updateExpression.push(`${attrName} = ${attrValue}`);
    expressionAttributeNames[attrName] = key;
    expressionAttributeValues[attrValue] = updates[key];
  });

  await docClient.send(new UpdateCommand({
    TableName: MINDMAPS_TABLE,
    Key: { id },
    UpdateExpression: `SET ${updateExpression.join(', ')}`,
    ExpressionAttributeNames: expressionAttributeNames,
    ExpressionAttributeValues: expressionAttributeValues
  }));
}

/**
 * Deletes a mindmap from DynamoDB
 * @param {string} id - The mindmap ID
 * @returns {Promise<boolean>} - True if deleted, false if not found
 */
async function deleteMindmap(id) {
  try {
    // Check if item exists first
    const existing = await getMindmap(id);
    if (!existing) {
      return false;
    }

    await docClient.send(new DeleteCommand({
      TableName: MINDMAPS_TABLE,
      Key: { id }
    }));

    return true;
  } catch (error) {
    console.error('Error deleting mindmap:', error);
    throw error;
  }
}

/**
 * Calls Claude on AWS Bedrock with retry logic and exponential backoff.
 * @param {string} prompt - The prompt to send to the model.
 * @param {string} systemPrompt - The system prompt for Claude.
 * @returns {Promise<object>} - The parsed JSON response from Claude.
 */
async function callClaude(prompt, systemPrompt = '') {
  const modelId = 'anthropic.claude-3-5-haiku-20241022-v1:0'; // Claude 3 Haiku
  
  const payload = {
    anthropic_version: 'bedrock-2023-05-31',
    max_tokens: 4000,
    temperature: 0.0,
    system: systemPrompt,
    messages: [
      {
        role: 'user',
        content: prompt
      }
    ]
  };

  const maxRetries = 3;
  let delay = 1000; // Start with a 1-second delay

  for (let i = 0; i < maxRetries; i++) {
    try {
      const command = new InvokeModelCommand({
        modelId: modelId,
        contentType: 'application/json',
        accept: 'application/json',
        body: JSON.stringify(payload)
      });

      const response = await bedrockClient.send(command);
      const responseBody = JSON.parse(new TextDecoder().decode(response.body));
      
      // Extract the text content from Claude's response
      const responseText = responseBody.content[0].text;
      
      // Try to parse as JSON
      try {
        return JSON.parse(responseText);
      } catch (parseError) {
        // If JSON parsing fails, try to extract JSON from the text
        const jsonMatch = responseText.match(/\{[\s\S]*\}/);
        if (jsonMatch) {
          return JSON.parse(jsonMatch[0]);
        }
        throw new Error('Could not parse JSON from Claude response');
      }

    } catch (error) {
      console.warn(`Bedrock API error (attempt ${i + 1}):`, error.message);
      
      // Check for throttling or service errors
      if (error.name === 'ThrottlingException' || error.name === 'ServiceException') {
        if (i < maxRetries - 1) {
          console.warn(`Retrying in ${delay / 1000}s...`);
          await new Promise(resolve => setTimeout(resolve, delay));
          delay *= 2; // Exponential backoff
          continue;
        }
      }
      
      // For the last retry or non-retryable errors, throw
      if (i === maxRetries - 1) {
        console.error('Error calling Bedrock Claude API after all retries:', error);
        throw error;
      }
    }
  }
  
  throw new Error('Bedrock Claude API call failed after multiple retries.');
}

/**
 * Extracts metadata from PDF text using Claude.
 * @param {string} pdfText - The full text content of the PDF.
 * @returns {Promise<object>} - An object containing title, authors, and date.
 */
async function extractMetadata(pdfText) {
  const systemPrompt = `You are a research paper analyzer. Extract the title, all authors, and publication date from research papers. Return only valid JSON with no additional text.`;
  
  const prompt = `Extract the title, all authors, and the publication date from the following research paper text. The date might be just a month and year, or more specific. Return only a JSON object with the following structure:
{
  "title": "paper title",
  "authors": ["author1", "author2"],
  "date": "publication date"
}

Text: 

${pdfText.substring(0, 4000)}`;

  return callClaude(prompt, systemPrompt);
}

/**
 * Generates a mind map from PDF text using Claude.
 * @param {string} pdfText - The full text content of the PDF.
 * @returns {Promise<object>} - A hierarchical JSON object for the D3 mind map.
 */
async function generateMindmap(pdfText) {
  const systemPrompt = `You are an expert at creating hierarchical mind maps from academic papers. Create structured JSON mind maps with up to 8 levels of depth. Each node must have: name, tooltip, section, pages, and optionally children. Return only valid JSON with no additional text.`;
  
  const prompt = `Analyze the following research paper text and create a hierarchical mind map summarizing its key concepts. The structure should be a nested JSON object with up to 8 levels but start with no more than 5. 

For each node, provide:
- 'name': concise topic name
- 'tooltip': three to five sentences, plain-english explanation, summarization of content
- 'section': the document section it belongs to (e.g., "Introduction", "2.1 Related Work")
- 'pages': a string with the source page number(s) (e.g., "3" or "5-7" - these must be factually accurate)
- 'children': array of child nodes (if applicable)

The root object should represent the paper's main theme and must have a 'children' array.

Return the response as a JSON object in this exact format:
{
  "name": "main topic",
  "tooltip": "explanation",
  "section": "section name", 
  "pages": "page numbers",
  "children": [
    {
      "name": "subtopic",
      "tooltip": "explanation",
      "section": "section name",
      "pages": "page numbers",
      "children": [...]
    }
  ]
}

Here is the text:

${pdfText}`;

  return callClaude(prompt, systemPrompt);
}

/**
 * Helper function to update a nested node in an object using a path array.
 * @param {object} obj - The object to traverse.
 * @param {Array<string|number>} path - The path to the node, e.g., ['children', 0, 'children', 1].
 * @param {object} updates - The new data to merge into the node.
 * @returns {boolean} - True if the update was successful, otherwise false.
 */
function updateNodeByPath(obj, path, updates) {
  let current = obj;
  for (let i = 0; i < path.length; i++) {
    const key = path[i];
    if (i === path.length - 1) {
      if (current && typeof current[key] !== 'undefined') {
        current[key] = { ...current[key], ...updates };
        return true;
      }
    }
    if (typeof current[key] === 'undefined') {
      return false; // Path does not exist
    }
    current = current[key];
  }
  return false;
}

// --- API ROUTES ---

// Serve the main page
app.get('/', (req, res) => {
  res.sendFile(path.join(__dirname, 'public', 'index.html'));
});

// Serve the admin page
app.get('/admin', (req, res) => {
  res.sendFile(path.join(__dirname, 'public', 'admin.html'));
});

// Admin login
app.post('/api/login', (req, res) => {
  const { password } = req.body;
  if (password === ADMIN_PASSWORD) {
    res.status(200).json({ success: true, message: 'Login successful' });
  } else {
    res.status(401).json({ success: false, message: 'Invalid password' });
  }
});

// Get all mind maps
app.get('/api/mindmaps', async (req, res) => {
  try {
    const mindmaps = await getAllMindmaps();
    res.status(200).json(mindmaps);
  } catch (error) {
    console.error('Error fetching mindmaps:', error);
    res.status(500).send('Error fetching mindmaps');
  }
});

// Delete a mind map
app.delete('/api/mindmaps/:id', async (req, res) => {
  try {
    const { id } = req.params;
    const deleted = await deleteMindmap(id);

    if (!deleted) {
      return res.status(404).send('Mindmap not found');
    }

    console.log('Mindmap deleted from DynamoDB with ID:', id);
    res.status(200).json({ success: true, message: 'Mindmap deleted successfully' });
  } catch (error) {
    console.error('Error deleting mindmap:', error);
    res.status(500).send('Error deleting mindmap');
  }
});

// Redo a node's description
app.post('/api/mindmaps/:id/redo-description', async (req, res) => {
  const { nodePath, nodeData } = req.body;
  const { id } = req.params;

  if (!nodePath || !nodeData) {
    return res.status(400).send('Missing nodePath or nodeData in request body.');
  }

  try {
    const mindmap = await getMindmap(id);
    if (!mindmap) return res.status(404).send('Mindmap not found.');

    const { pdfText } = mindmap;
    if (!pdfText) return res.status(500).send('PDF text not found for this mindmap.');

    const systemPrompt = `You are an expert at explaining academic concepts. Provide clear, concise explanations in plain English. Return only valid JSON with no additional text.`;
    
    const prompt = `Given the full text of a research paper, please rewrite a short, plain-english "tooltip" description for the specific concept: "${nodeData.name}". The description should explain the concept in the context of the paper. Keep it concise.

Return only a JSON object in this format:
{
  "tooltip": "your explanation here"
}

Full Paper Text:
${pdfText}`;

    const result = await callClaude(prompt, systemPrompt);
    const tooltip = result.tooltip;

    let fullMindmapData = mindmap.mindmapData;
    const success = updateNodeByPath(fullMindmapData, nodePath, { tooltip });

    if (success) {
      await updateMindmap(id, { mindmapData: fullMindmapData });
      console.log(`Description updated for node in mindmap ${id}`);
      res.status(200).json({ success: true, newTooltip: tooltip });
    } else {
      res.status(404).send('Node path not found in mindmap data.');
    }
  } catch (error) {
    console.error('Error redoing node description:', error);
    res.status(500).send('Failed to redo node description.');
  }
});

// Remake a subtree from a specific node
app.post('/api/mindmaps/:id/remake-subtree', async (req, res) => {
  const { nodePath, nodeData } = req.body;
  const { id } = req.params;

  if (!nodePath || !nodeData) {
    return res.status(400).send('Missing nodePath or nodeData in request body.');
  }

  try {
    const mindmap = await getMindmap(id);
    if (!mindmap) return res.status(404).send('Mindmap not found.');

    const { pdfText } = mindmap;
    if (!pdfText) return res.status(500).send('PDF text not found for this mindmap.');

    const currentDepth = (nodePath.length / 2);
    const maxDepth = 4;
    if (currentDepth >= maxDepth) {
      return res.status(400).json({ success: false, message: 'Cannot remake subtree from the deepest level.' });
    }

    const systemPrompt = `You are an expert at creating hierarchical mind maps from academic papers. Create structured JSON mind maps. Return only valid JSON with no additional text.`;
    
    const prompt = `From the research paper provided, expand on the specific topic: "${nodeData.name}". Create a hierarchical list of sub-topics that would fall under this main topic, structured as a mind map. 

The root of this new map should be "${nodeData.name}", and it can have children and grandchildren. For each node, provide:
- 'name': topic name
- 'tooltip': plain-english explanation  
- 'section': document section
- 'pages': page numbers

Return the response as a JSON object in this format:
{
  "name": "${nodeData.name}",
  "tooltip": "explanation",
  "section": "section name",
  "pages": "page numbers", 
  "children": [
    {
      "name": "subtopic",
      "tooltip": "explanation", 
      "section": "section name",
      "pages": "page numbers",
      "children": [...]
    }
  ]
}

Full Paper Text:
${pdfText}`;

    const newSubtree = await callClaude(prompt, systemPrompt);

    let fullMindmapData = mindmap.mindmapData;
    const success = updateNodeByPath(fullMindmapData, nodePath, { children: newSubtree.children || [] });

    if (success) {
      await updateMindmap(id, { mindmapData: fullMindmapData });
      console.log(`Subtree remade for node in mindmap ${id}`);
      res.status(200).json({ success: true, newChildren: newSubtree.children || [] });
    } else {
      res.status(404).send('Node path not found in mindmap data.');
    }
  } catch (error) {
    console.error('Error remaking subtree:', error);
    res.status(500).send('Failed to remake subtree.');
  }
});

// Go one level deeper from a leaf node
app.post('/api/mindmaps/:id/go-deeper', async (req, res) => {
    const { nodePath, nodeData } = req.body;
    const { id } = req.params;

    if (!nodePath || !nodeData) {
        return res.status(400).send('Missing nodePath or nodeData.');
    }

    try {
        const mindmap = await getMindmap(id);
        if (!mindmap) return res.status(404).send('Mindmap not found.');

        const { pdfText } = mindmap;
        if (!pdfText) return res.status(500).send('PDF text not found.');

        const systemPrompt = `You are an expert at expanding academic topics into subtopics. Create structured JSON arrays. Return only valid JSON with no additional text.`;
        
        const prompt = `Based on the provided research paper, expand on the topic "${nodeData.name}". Generate a new list of direct sub-topics (children). 

For each child, provide:
- 'name': topic name
- 'tooltip': plain-english explanation
- 'section': document section  
- 'pages': page numbers

Return this as a JSON object with a single 'children' array:
{
  "children": [
    {
      "name": "subtopic name",
      "tooltip": "explanation",
      "section": "section name", 
      "pages": "page numbers"
    }
  ]
}

Full Paper Text:
${pdfText}`;

        const result = await callClaude(prompt, systemPrompt);
        const newChildren = result.children || [];

        let fullMindmapData = mindmap.mindmapData;
        const success = updateNodeByPath(fullMindmapData, nodePath, { children: newChildren });

        if (success) {
            await updateMindmap(id, { mindmapData: fullMindmapData });
            console.log(`Went deeper from node in mindmap ${id}`);
            res.status(200).json({ success: true, newChildren });
        } else {
            res.status(404).send('Node path not found in mindmap data.');
        }
    } catch (error) {
        console.error('Error going deeper:', error);
        res.status(500).send('Failed to go deeper.');
    }
});

// PDF Upload and Processing
app.post('/api/upload', upload.single('pdf'), async (req, res) => {
  if (!req.file) {
    return res.status(400).send('No file uploaded.');
  }

  try {
    console.log('Processing uploaded PDF:', req.file.originalname);
    const pdfBuffer = req.file.buffer;
    const data = await pdf(pdfBuffer);
    const pdfText = data.text;

    // Process metadata extraction and mindmap generation sequentially
    console.log('Extracting metadata...');
    const metadata = await extractMetadata(pdfText);
    console.log('Generating mindmap...');
    const mindmapData = await generateMindmap(pdfText);

    console.log('Successfully generated data from Claude.');

    const newMindmap = {
      filename: req.file.originalname,
      title: metadata.title,
      authors: metadata.authors,
      date: metadata.date,
      mindmapData: mindmapData,
      pdfText: pdfText,
    };

    const mindmapId = await createMindmap(newMindmap);
    console.log('Mindmap saved to DynamoDB with ID:', mindmapId);

    res.status(201).json({ success: true, message: 'PDF processed and mind map created!', mindmapId });

  } catch (error) {
    console.error('Error processing PDF:', error);
    res.status(500).send('Failed to process PDF.');
  }
});

// --- START SERVER ---
app.listen(PORT, () => {
  console.log(`Server is running on http://localhost:${PORT}`);
});
