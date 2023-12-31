import { patternsJson } from "./patterns.js";

let generationCount = 0;
let patternName = "";

document.addEventListener("DOMContentLoaded", () => {
    const size = 20;
    let grid = createGrid(size);
    let interval: number | NodeJS.Timeout;
    draw(grid);
    let activeCellsCount = 0;

    document.getElementById("startButton")?.addEventListener("click", () => {
        interval = setInterval(() => {
            grid = nextGeneration(grid);
            activeCellsCount = grid.flat().filter((cell) => cell).length;
            document.getElementById("activeCellsCount")!.innerText =
                activeCellsCount.toString();
            generationCount++;
            document.getElementById("generationCount")!.innerText =
                generationCount.toString();
            draw(grid);
        }, 300);
    });

    document.getElementById("stopButton")?.addEventListener("click", () => {
        clearInterval(interval as any);
    });

    document.getElementById("nextStepButton")?.addEventListener("click", () => {
        grid = nextGeneration(grid);
        activeCellsCount = grid.flat().filter((cell) => cell).length;
        document.getElementById("activeCellsCount")!.innerText =
            activeCellsCount.toString();
        generationCount++;
        document.getElementById("generationCount")!.innerText =
            generationCount.toString();
        draw(grid);
    });

    document.getElementById("resetButton")?.addEventListener("click", () => {
        draw(grid);
    });

    draw(grid);
});

function createGrid(size: number): boolean[][] {
    const grid: boolean[][] = [];
    const patterns = patternsJson.patterns;

    // Create empty grid
    for (let i = 0; i < size; i++) {
        const row: boolean[] = [];
        for (let j = 0; j < size; j++) {
            row.push(false);
        }
        grid.push(row);
    }

    // Randomly select a pattern
    const patternObj = patterns[Math.floor(Math.random() * patterns.length)];
    const pattern = patternObj.pattern; // Extract the pattern from the selected object
    patternName = patternObj.name;

    // Apply pattern to grid at a random position
    const startX = Math.floor(Math.random() * (size - pattern.length));
    const startY = Math.floor(Math.random() * (size - pattern[0].length));
    for (let i = 0; i < pattern.length; i++) {
        for (let j = 0; j < pattern[0].length; j++) {
            grid[startX + i][startY + j] = pattern[i][j];
        }
    }

    return grid;
}

function nextGeneration(grid: boolean[][]): boolean[][] {
    const size = grid.length;
    const newGrid = createGrid(size);

    for (let i = 0; i < size; i++) {
        for (let j = 0; j < size; j++) {
            const neighbors = countNeighbors(grid, i, j);
            if (grid[i][j] && (neighbors === 2 || neighbors === 3)) {
                newGrid[i][j] = true;
            } else if (!grid[i][j] && neighbors === 3) {
                newGrid[i][j] = true;
            }
        }
    }

    return newGrid;
}

function countNeighbors(grid: boolean[][], x: number, y: number): number {
    let count = 0;
    for (let i = -1; i <= 1; i++) {
        for (let j = -1; j <= 1; j++) {
            if (i === 0 && j === 0) continue;
            const xi = (x + i + grid.length) % grid.length;
            const yj = (y + j + grid.length) % grid.length;
            if (grid[xi][yj]) count++;
        }
    }
    return count;
}

function draw(grid: boolean[][]): void {
    const container = document.getElementById("grid-container")!;
    container.innerHTML = "";

    let activeCellsCount = 0;

    grid.forEach((row) => {
        row.forEach((cell) => {
            const cellDiv = document.createElement("div");
            cellDiv.className = "cell";
            if (cell) {
                cellDiv.classList.add("alive");
                activeCellsCount++;
            }
            container.appendChild(cellDiv);
        });
    });

    document.getElementById("activeCellsCount")!.innerText =
        activeCellsCount.toString();
    document.getElementById("generationCount")!.innerText =
        generationCount.toString();
    document.getElementById("patternName")!.innerText = patternName.toString();
}
