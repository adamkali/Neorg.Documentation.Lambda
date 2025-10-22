-- Simple lua-utils shim for Neorg compatibility
-- This provides basic functionality that Neorg expects from lua-utils

local M = {}

-- Basic table utility functions
function M.tbl_extend(behavior, target, ...)
    local sources = {...}
    
    if behavior == "force" then
        for _, source in ipairs(sources) do
            if type(source) == "table" then
                for k, v in pairs(source) do
                    target[k] = v
                end
            end
        end
    elseif behavior == "keep" then
        for _, source in ipairs(sources) do
            if type(source) == "table" then
                for k, v in pairs(source) do
                    if target[k] == nil then
                        target[k] = v
                    end
                end
            end
        end
    end
    
    return target
end

function M.tbl_deep_extend(behavior, target, ...)
    local sources = {...}
    
    for _, source in ipairs(sources) do
        if type(source) == "table" then
            for k, v in pairs(source) do
                if type(v) == "table" and type(target[k]) == "table" then
                    M.tbl_deep_extend(behavior, target[k], v)
                elseif behavior == "force" or target[k] == nil then
                    target[k] = v
                end
            end
        end
    end
    
    return target
end

function M.tbl_keys(t)
    local keys = {}
    for k, _ in pairs(t) do
        table.insert(keys, k)
    end
    return keys
end

function M.tbl_values(t)
    local values = {}
    for _, v in pairs(t) do
        table.insert(values, v)
    end
    return values
end

function M.tbl_count(t)
    local count = 0
    for _ in pairs(t) do
        count = count + 1
    end
    return count
end

-- String utilities
function M.split(str, delimiter)
    local result = {}
    local pattern = "(.-)" .. (delimiter or "%s") .. "()"
    local lastPos = 1
    
    for part, pos in str:gmatch(pattern) do
        table.insert(result, part)
        lastPos = pos
    end
    
    table.insert(result, str:sub(lastPos))
    return result
end

function M.trim(str)
    return str:match("^%s*(.-)%s*$")
end

-- File system utilities (basic)
function M.is_file(path)
    local file = io.open(path, "r")
    if file then
        file:close()
        return true
    end
    return false
end

function M.is_directory(path)
    -- Basic directory check using package.config
    local sep = package.config:sub(1,1)
    local file = io.open(path .. sep .. ".", "r")
    if file then
        file:close()
        return true
    end
    return false
end

-- Basic path utilities
function M.join_paths(...)
    local sep = package.config:sub(1,1)
    local paths = {...}
    local result = paths[1] or ""
    
    for i = 2, #paths do
        if paths[i] and paths[i] ~= "" then
            if result:sub(-1) ~= sep and paths[i]:sub(1,1) ~= sep then
                result = result .. sep .. paths[i]
            elseif result:sub(-1) == sep and paths[i]:sub(1,1) == sep then
                result = result .. paths[i]:sub(2)
            else
                result = result .. paths[i]
            end
        end
    end
    
    return result
end

-- Neorg-specific utilities
function M.inline_pcall(func, ...)
    local ok, result = pcall(func, ...)
    if ok then
        return result
    else
        -- Return nil on error instead of raising
        return nil
    end
end

-- Additional utilities that might be needed by Neorg
function M.tbl_isempty(t)
    return next(t) == nil
end

function M.tbl_islist(t)
    if type(t) ~= "table" then
        return false
    end
    
    local i = 0
    for _ in pairs(t) do
        i = i + 1
        if t[i] == nil then
            return false
        end
    end
    return true
end

function M.tbl_map(func, t)
    local result = {}
    for k, v in pairs(t) do
        result[k] = func(v)
    end
    return result
end

function M.tbl_filter(func, t)
    local result = {}
    for k, v in pairs(t) do
        if func(v) then
            result[k] = v
        end
    end
    return result
end

-- Async/callback utilities
function M.schedule_wrap(func)
    return function(...)
        local args = {...}
        return vim.schedule(function()
            func(unpack(args))
        end)
    end
end

function M.defer_fn(func, timeout)
    return vim.defer_fn(func, timeout)
end

-- Additional Vim utilities that might be needed
function M.wrap(func, ...)
    local args = {...}
    return function(...)
        local new_args = {...}
        local combined_args = {}
        for i, v in ipairs(args) do
            table.insert(combined_args, v)
        end
        for i, v in ipairs(new_args) do
            table.insert(combined_args, v)
        end
        return func(unpack(combined_args))
    end
end

function M.once(func)
    local called = false
    local result
    return function(...)
        if not called then
            called = true
            result = func(...)
        end
        return result
    end
end

-- Additional string utilities
function M.startswith(str, prefix)
    return str:sub(1, #prefix) == prefix
end

function M.endswith(str, suffix)
    return str:sub(-#suffix) == suffix
end

return M