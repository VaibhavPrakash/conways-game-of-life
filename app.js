const patternsJson = {
    patterns: [
        {
            name: "Blinker",
            pattern: [
                [false, true, false],
                [false, true, false],
                [false, true, false],
            ],
        },
        {
            name: "Glider",
            pattern: [
                [false, true, false],
                [false, false, true],
                [true, true, true],
            ],
        },
        {
            name: "Toad",
            pattern: [
                [false, true, true, true],
                [true, true, true, false],
            ],
        },
        {
            name: "Beacon",
            pattern: [
                [true, true, false, false],
                [true, true, false, false],
                [false, false, true, true],
                [false, false, true, true],
            ],
        },
    ],
};

let generationCount = 0;


document.addEventListener("DOMContentLoaded", () => {
    const size = 20;
    let grid = createGrid(size);
    let interval;
    draw(grid);
    let activeCellsCount = 0;

    document.getElementById("startButton")?.addEventListener("click", () => {
        interval = setInterval(() => {
            grid = nextGeneration(grid);
            activeCellsCount = grid.flat().filter((cell) => cell).length;
            document.getElementById("activeCellsCount").innerText =
                activeCellsCount.toString();
            generationCount++;
            document.getElementById("generationCount").innerText =
                generationCount.toString();
            draw(grid);
        }, 300);
    });

    document.getElementById("stopButton")?.addEventListener("click", () => {
        clearInterval(interval);
    });

    document.getElementById("nextStepButton")?.addEventListener("click", () => {
        grid = nextGeneration(grid);
        activeCellsCount = grid.flat().filter((cell) => cell).length;
        document.getElementById("activeCellsCount").innerText =
            activeCellsCount.toString();
        generationCount++;
        document.getElementById("generationCount").innerText =
            generationCount.toString();
        draw(grid);
    });

    document.getElementById("resetButton")?.addEventListener("click", () => {
        grid = createGrid(size);
        draw(grid);
    });

    draw(grid);
});


function createGrid(size) {
    const grid = [];
    const patterns = patternsJson.patterns;

    for (let i = 0; i < size; i++) {
        const row = [];
        for (let j = 0; j < size; j++) {
            row.push(false);
        }
        grid.push(row);
    }

    const patternObj = patterns[Math.floor(Math.random() * patterns.length)];
    const pattern = patternObj.pattern;

    const startX = Math.floor(Math.random() * (size - pattern.length));
    const startY = Math.floor(Math.random() * (size - pattern[0].length));
    for (let i = 0; i < pattern.length; i++) {
        for (let j = 0; j < pattern[0].length; j++) {
            grid[startX + i][startY + j] = pattern[i][j];
        }
    }

    return grid;
}

function nextGeneration(grid) {
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

function countNeighbors(grid, x, y) {
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

function draw(grid) {
    const container = document.getElementById("grid-container");
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

    document.getElementById("activeCellsCount").innerText =
        activeCellsCount.toString();
    document.getElementById("generationCount").innerText =
        generationCount.toString();
}
