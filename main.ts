document.addEventListener("DOMContentLoaded", () => {
    const size = 20;
    let grid = createGrid(size);
    let interval: number | NodeJS.Timeout ;
  
    document.getElementById('startButton')?.addEventListener('click', () => {
      interval = setInterval(() => {
        grid = nextGeneration(grid);
        draw(grid);
      }, 100);
    });
  
    document.getElementById('stopButton')?.addEventListener('click', () => {
      clearInterval(interval);
    });
  
    draw(grid);
  });
  
  function createGrid(size: number): boolean[][] {
    const grid: boolean[][] = [];
  
    for (let i = 0; i < size; i++) {
      const row: boolean[] = [];
      for (let j = 0; j < size; j++) {
        row.push(false);
      }
      grid.push(row);
    }
  
    // Initialize some live cells
    grid[5][5] = true;
    grid[5][6] = true;
    grid[5][7] = true;
  
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
    container.innerHTML = '';
  
    grid.forEach(row => {
      row.forEach(cell => {
        const cellDiv = document.createElement('div');
        cellDiv.className = 'cell';
        if (cell) cellDiv.classList.add('alive');
        container.appendChild(cellDiv);
      });
    });
  }
  