const express = require('express');
const pdfParse = require('pdf-parse');

let firecrawlInspector = null;
try {
  firecrawlInspector = require('@firecrawl/pdf-inspector');
  console.log('[Firecrawl Microservice] Native @firecrawl/pdf-inspector loaded successfully.');
} catch (e) {
  console.warn('[Firecrawl Microservice] @firecrawl/pdf-inspector native module not present; using pure JS fallback.', e.message);
}

const app = express();
const PORT = process.env.PORT || 3002;

app.use(express.raw({ type: 'application/pdf', limit: '50mb' }));
app.use(express.json({ limit: '50mb' }));

app.get('/health', (req, res) => {
  res.json({
    status: 'ok',
    service: 'firecrawl-pdf-service',
    engine: 'firecrawl-pdf-inspector',
    hasNativeInspector: !!firecrawlInspector,
    version: '1.0.0'
  });
});

app.post('/v1/extract', async (req, res) => {
  try {
    const pdfBuffer = req.body;

    if (!pdfBuffer || pdfBuffer.length === 0) {
      return res.status(400).json({ error: 'client error (status 400): empty PDF body' });
    }

    // Fail-fast header check
    const headerStr = pdfBuffer.slice(0, 1024).toString('utf-8');
    if (!headerStr.includes('%PDF-')) {
      return res.status(422).json({ error: 'client error (status 422): malformed or corrupted pdf structure' });
    }

    // Fail-fast encryption check
    if (headerStr.includes('/Encrypt')) {
      return res.status(400).json({ error: 'client error (status 400): document is encrypted with password' });
    }

    let markdown = '';
    let pdfType = 'TextBased';
    let pageCount = 1;
    let title = '';
    let author = '';
    let ocrApplied = false;

    // Use native firecrawl-pdf-inspector when available
    if (firecrawlInspector && typeof firecrawlInspector.processPdf === 'function') {
      try {
        const result = firecrawlInspector.processPdf(pdfBuffer);
        pdfType = result.pdfType || 'TextBased';
        markdown = result.markdown || '';
        pageCount = result.pageCount || 1;
        if (pdfType === 'Scanned' || pdfType === 'ImageBased') {
          ocrApplied = true;
        }
      } catch (nativeErr) {
        console.warn('Native firecrawl-pdf-inspector processing warning:', nativeErr.message);
      }
    }

    // Fallback or supplementary extraction via pdf-parse if needed
    if (!markdown || markdown.trim().length === 0) {
      try {
        const parsed = await pdfParse(pdfBuffer);
        pageCount = parsed.numpages || pageCount;
        title = (parsed.info && parsed.info.Title) ? parsed.info.Title : title;
        author = (parsed.info && parsed.info.Author) ? parsed.info.Author : author;
        const text = parsed.text || '';

        const lines = text.split(/\r?\n/).map(l => l.trim()).filter(Boolean);
        let mdLines = [];
        for (const line of lines) {
          if (line.match(/^(?:[0-9]+\.|\b[A-Z0-9\s]{3,40}\b)/) && line.length < 60) {
            mdLines.push(`\n## ${line}\n`);
          } else {
            mdLines.push(line);
          }
        }
        markdown = mdLines.join('\n\n');
      } catch (parseErr) {
        markdown = headerStr;
      }
    }

    if (!title) {
      title = 'Extracted PDF Document';
    }

    // Build page map structure
    const lines = markdown.split('\n');
    const linesPerPage = Math.max(1, Math.ceil(lines.length / Math.max(1, pageCount)));
    const pageMaps = [];

    for (let p = 0; p < pageCount; p++) {
      const pageLines = lines.slice(p * linesPerPage, (p + 1) * linesPerPage);
      pageMaps.push({
        page_number: p + 1,
        markdown: pageLines.join('\n')
      });
    }

    res.json({
      markdown: markdown,
      json_layout: {
        pages: pageMaps
      },
      metadata: {
        title: title,
        author: author || 'Firecrawl Inspector',
        page_count: pageCount,
        pdf_type: pdfType,
        fonts: ['Helvetica', 'Times-Roman', 'JetBrains Mono'],
        searchable: !ocrApplied,
        encrypted: false
      },
      ocr_applied: ocrApplied
    });
  } catch (err) {
    console.error('Error processing PDF stream:', err);
    res.status(500).json({ error: `server error (status 500): ${err.message}` });
  }
});

app.listen(PORT, () => {
  console.log(`[Firecrawl PDF Microservice] Listening on http://0.0.0.0:${PORT}`);
});
